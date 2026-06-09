# SQLite3 IndexedDB VFS for Go

This package provides a Virtual File System (VFS) implementation for [go-sqlite3](https://github.com/ncruces/go-sqlite3/) that persists data to IndexedDB in WebAssembly environments.

## Overview

The IndexedDB VFS allows SQLite databases to be stored in the browser's IndexedDB, providing persistence across browser sessions. In non-WebAssembly environments, it falls back to an in-memory database.

This implementation is similar to the [memdb](https://github.com/ncruces/go-sqlite3/tree/main/vfs/memdb) VFS, but instead of storing data in memory, it persists data to IndexedDB.

## Installation

```bash
go get github.com/justwasm/sqlite3-vfs-idb
```

## Usage

```go
package main

import (
	"database/sql"
	"log"

	_ "github.com/justwasm/sqlite3-vfs-idb"
	"github.com/ncruces/go-sqlite3"
)

func main() {
	// Open a database with the IndexedDB VFS
	db, err := sql.Open("sqlite3", "file:mydatabase.db?vfs=idb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Use the database as usual
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	// Insert data
	_, err = db.Exec("INSERT INTO users (name) VALUES (?)", "John Doe")
	if err != nil {
		log.Fatal(err)
	}

	// Query data
	rows, err := db.Query("SELECT id, name FROM users")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatal(err)
		}
		log.Printf("User: %d, %s", id, name)
	}
}
```

## How It Works

In WebAssembly environments:
- The VFS uses the browser's IndexedDB to store SQLite database files
- Each file is stored as a record in the IndexedDB store
- File operations are translated to IndexedDB operations

In non-WebAssembly environments:
- The VFS falls back to the in-memory VFS provided by go-sqlite3
- This ensures that code compiles and runs outside of browsers, but without persistence

## Example

An example of using the IndexedDB VFS in a WebAssembly environment is provided in the `examples/basic` directory. The example demonstrates how to:

1. Open a database with the IndexedDB VFS
2. Create a table
3. Insert data
4. Query data

### Running the Example

To run the example:

1. Navigate to the `examples/basic` directory
2. Run `make run` to build the WebAssembly binary and start the server
3. Open http://localhost:8080 in your browser
4. Click the "Run SQLite Example" button to execute the example

The example includes:

- `main.go`: The Go code that uses the IndexedDB VFS
- `index.html`: The HTML page that loads and runs the WebAssembly
- `server.go`: A simple HTTP server to serve the example
- `Makefile`: Simplifies building and running the example

## License

[MIT License](LICENSE)
