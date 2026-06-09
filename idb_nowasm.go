//go:build !js || !wasm

package idb

import (
	"log/slog"

	"github.com/ncruces/go-sqlite3/vfs"
)

func init() {
	// For non-WASM builds, we fall back to an in-memory database.
	// This ensures that the code compiles and runs outside of the browser,
	// but without persistence.
	vfs.Register("idb", vfs.Find("memdb"))
	slog.Info("Registered in-memory VFS for non-WASM environment")
}
