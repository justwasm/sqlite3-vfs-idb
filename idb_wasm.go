//go:build js && wasm

package idb

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"syscall/js"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
)

func init() {
	js.Global().Get("eval").Invoke(idbJS)
	vfs.Register("idb", &idbVFS{})
}

var (
	jsPromise    = js.Global().Get("Promise")
	jsUint8Array = js.Global().Get("Uint8Array")
)

// idbVFS implements the VFS interface for IndexedDB.
type idbVFS struct{}

// idbFile implements the VFS File interface for a file in IndexedDB.
type idbFile struct {
	name   string
	flags  vfs.OpenFlag
	data   *bytes.Buffer
	mu     sync.Mutex
	locked bool
}

func (f *idbFile) call(method string, args ...any) (js.Value, error) {
	// This function handles calls to the JavaScript VFS functions and waits for the promise to resolve.
	resCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	sqliteVFS := js.Global().Get("sqliteVFS")
	if !sqliteVFS.Truthy() {
		return js.Value{}, sqlite3.IOERR
	}

	p := sqliteVFS.Call(method, args...)
	p.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		resCh <- args[0]
		return nil
	}))
	p.Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
		errCh <- js.Error{Value: args[0]}
		return nil
	}))

	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return js.Value{}, err
	}
}

// Open implements the VFS interface.
func (v *idbVFS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	f := &idbFile{
		name:  name,
		flags: flags,
		data:  bytes.NewBuffer(nil),
	}

	// For read operations, try to load existing data from IndexedDB.
	if flags&vfs.OPEN_READONLY != 0 || flags&vfs.OPEN_READWRITE != 0 {
		val, err := f.call("getFile", name)
		if err != nil {
			return nil, 0, sqlite3.IOERR_READ
		}
		if val.Truthy() && !val.IsNull() {
			// Copy JS Uint8Array to Go byte slice
			jsData := js.Global().Get("Uint8Array").New(val)
			goBytes := make([]byte, jsData.Get("length").Int())
			js.CopyBytesToGo(goBytes, jsData)
			f.data = bytes.NewBuffer(goBytes)
		} else if flags&vfs.OPEN_CREATE != 0 {
			// File does not exist, but create flag is set.
			// The buffer is already empty, which is correct.
		} else {
			// File does not exist and no create flag.
			return nil, 0, sqlite3.CANTOPEN
		}
	}

	return f, flags, nil
}

// Delete implements the VFS interface.
func (v *idbVFS) Delete(name string, dirSync bool) error {
	f := &idbFile{name: name}
	_, err := f.call("deleteFile", name)
	if err != nil {
		return sqlite3.IOERR_DELETE
	}
	return nil
}

// Access implements the VFS interface.
func (v *idbVFS) Access(name string, flags vfs.AccessFlag) (bool, error) {
	f := &idbFile{name: name}
	val, err := f.call("getFile", name)
	if err != nil {
		// An error in JS might mean we can't access it, but let's check flags
		if flags == vfs.ACCESS_EXISTS {
			return false, nil // Assume it doesn't exist if there's an error
		}
		return false, sqlite3.IOERR_ACCESS
	}

	exists := val.Truthy() && !val.IsNull()

	switch flags {
	case vfs.ACCESS_EXISTS:
		return exists, nil
	case vfs.ACCESS_READWRITE, vfs.ACCESS_READ:
		return exists, nil // If it exists, we assume it's readable/writable for simplicity
	default:
		return false, nil
	}
}

// FullPathname implements the VFS interface.
func (v *idbVFS) FullPathname(name string) (string, error) {
	return name, nil
}

// Close implements the File interface.
func (f *idbFile) Close() error {
	// If the file was opened for writing, flush to IndexedDB on close.
	if f.flags&vfs.OPEN_READWRITE != 0 || f.flags&vfs.OPEN_CREATE != 0 {
		return f.Sync(0)
	}
	f.data = nil // Release memory
	return nil
}

// Read implements the File interface.
func (f *idbFile) Read(p []byte) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data.Read(p)
}

// ReadAt implements the File interface.
func (f *idbFile) ReadAt(p []byte, off int64) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if off >= int64(f.data.Len()) {
		return 0, io.EOF
	}

	reader := bytes.NewReader(f.data.Bytes())
	return reader.ReadAt(p, off)
}

// Write implements the File interface.
func (f *idbFile) Write(p []byte) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data.Write(p)
}

// WriteAt implements the File interface.
func (f *idbFile) WriteAt(p []byte, off int64) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Grow buffer if necessary
	if off+int64(len(p)) > int64(f.data.Len()) {
		f.data.Grow(int(off + int64(len(p)) - int64(f.data.Len())))
		// This is a bit tricky, we need to pad with zeros
		// A simple write at the end would be easier
	}

	// A simpler way for a buffer is to overwrite
	// This is not efficient, but robust for a bytes.Buffer
	currentData := f.data.Bytes()
	if off > int64(len(currentData)) {
		// Cannot seek past the end for writing this way
		return 0, io.EOF
	}

	n = copy(currentData[off:], p)

	// If we wrote past the end, we need to handle that
	if int64(n) < int64(len(p)) {
		f.data.Write(p[n:])
	}

	return len(p), nil
}

// Seek implements the File interface.
func (f *idbFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		// This is tricky as bytes.Buffer doesn't have a concept of current position.
		// We'll treat this as not supported for now.
		return 0, sqlite3.IOERR
	case io.SeekEnd:
		newOffset = int64(f.data.Len()) + offset
	}

	// This is also tricky; we just return the new offset
	// The next Read/WriteAt call will use it.
	return newOffset, nil
}

// Truncate implements the File interface.
func (f *idbFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data.Truncate(int(size))
	return nil
}

// Sync implements the File interface, persisting data to IndexedDB.
func (f *idbFile) Sync(flags vfs.SyncFlag) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Copy Go byte slice to JS Uint8Array
	jsData := jsUint8Array.New(f.data.Len())
	js.CopyBytesToJS(jsData, f.data.Bytes())

	_, err := f.call("putFile", f.name, jsData)
	if err != nil {
		slog.Error("Sync failed", "name", f.name, "err", err)
		return sqlite3.IOERR_FSYNC
	}
	return nil
}

// Size implements the File interface.
func (f *idbFile) Size() (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(f.data.Len()), nil
}

// Lock implements the File interface.
func (f *idbFile) Lock(lock vfs.LockLevel) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked = true
	return nil
}

// Unlock implements the File interface.
func (f *idbFile) Unlock(lock vfs.LockLevel) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked = false
	return nil
}

// CheckReservedLock implements the File interface.
func (f *idbFile) CheckReservedLock() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.locked, nil
}

// DeviceCharacteristics implements the File interface.
func (f *idbFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	// Return reasonable capabilities for an IndexedDB-based file system
	return vfs.IOCAP_ATOMIC | vfs.IOCAP_SAFE_APPEND | vfs.IOCAP_SEQUENTIAL
}

// SectorSize implements the File interface.
func (f *idbFile) SectorSize() int {
	return 4096 // Standard sector size for most file systems
}
