//go:build js && wasm

package idb

import (
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

var jsUint8Array = js.Global().Get("Uint8Array")

// idbVFS implements the VFS interface for IndexedDB.
type idbVFS struct{}

// idbFile implements the VFS File interface for a file in IndexedDB.
type idbFile struct {
	name   string
	flags  vfs.OpenFlag
	data   []byte
	mu     sync.RWMutex
	locked bool
}

func (f *idbFile) call(method string, args ...any) (js.Value, error) {
	resCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	sqliteVFS := js.Global().Get("sqliteVFS")
	if !sqliteVFS.Truthy() {
		return js.Value{}, sqlite3.IOERR
	}

	p := sqliteVFS.Call(method, args...)

	then := js.FuncOf(func(this js.Value, args []js.Value) any {
		resCh <- args[0]
		return nil
	})
	catch := js.FuncOf(func(this js.Value, args []js.Value) any {
		errCh <- js.Error{Value: args[0]}
		return nil
	})

	p.Call("then", then)
	p.Call("catch", catch)

	select {
	case res := <-resCh:
		then.Release()
		catch.Release()
		return res, nil
	case err := <-errCh:
		then.Release()
		catch.Release()
		return js.Value{}, err
	}
}

// Open implements the VFS interface.
func (v *idbVFS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	f := &idbFile{
		name:  name,
		flags: flags,
	}

	// For read operations, try to load existing data from IndexedDB.
	if flags&vfs.OPEN_READONLY != 0 || flags&vfs.OPEN_READWRITE != 0 {
		val, err := f.call("getFile", name)
		if err != nil {
			return nil, 0, sqlite3.IOERR_READ
		}
		if val.Truthy() && !val.IsNull() {
			// Copy JS Uint8Array to Go byte slice
			jsData := jsUint8Array.New(val)
			goBytes := make([]byte, jsData.Get("length").Int())
			js.CopyBytesToGo(goBytes, jsData)
			f.data = goBytes
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
		if flags == vfs.ACCESS_EXISTS {
			return false, nil
		}
		return false, sqlite3.IOERR_ACCESS
	}

	exists := val.Truthy() && !val.IsNull()

	switch flags {
	case vfs.ACCESS_EXISTS:
		return exists, nil
	case vfs.ACCESS_READWRITE, vfs.ACCESS_READ:
		return exists, nil
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

// ReadAt implements the File interface.
func (f *idbFile) ReadAt(p []byte, off int64) (n int, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n = copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt implements the File interface.
func (f *idbFile) WriteAt(p []byte, off int64) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	end := off + int64(len(p))
	if end > int64(len(f.data)) {
		// Extend the slice with zeroes to accommodate the write.
		newData := make([]byte, end)
		copy(newData, f.data)
		f.data = newData
	}

	n = copy(f.data[off:], p)
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
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
		return 0, sqlite3.IOERR
	case io.SeekEnd:
		newOffset = int64(len(f.data)) + offset
	}

	return newOffset, nil
}

// Truncate implements the File interface.
func (f *idbFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if size >= int64(len(f.data)) {
		// Growing: zero-extend.
		newData := make([]byte, size)
		copy(newData, f.data)
		f.data = newData
	} else {
		// Shrinking: truncate.
		f.data = f.data[:size]
	}
	return nil
}

// Sync implements the File interface, persisting data to IndexedDB.
func (f *idbFile) Sync(flags vfs.SyncFlag) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Copy Go byte slice to JS Uint8Array
	jsData := jsUint8Array.New(len(f.data))
	js.CopyBytesToJS(jsData, f.data)

	_, err := f.call("putFile", f.name, jsData)
	if err != nil {
		slog.Error("Sync failed", "name", f.name, "err", err)
		return sqlite3.IOERR_FSYNC
	}
	return nil
}

// Size implements the File interface.
func (f *idbFile) Size() (int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return int64(len(f.data)), nil
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
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.locked, nil
}

// DeviceCharacteristics implements the File interface.
func (f *idbFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return vfs.IOCAP_ATOMIC | vfs.IOCAP_SAFE_APPEND | vfs.IOCAP_SEQUENTIAL
}

// SectorSize implements the File interface.
func (f *idbFile) SectorSize() int {
	return 4096
}
