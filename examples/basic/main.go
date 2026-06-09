package main

import (
	"log"
	"syscall/js"
)

import "database/sql"
import _ "github.com/ncruces/go-sqlite3/driver"
import _ "github.com/ncruces/go-sqlite3/embed"
import _ "github.com/justwasm/sqlite3-vfs-idb"

func main() {
	// Set up a done channel to wait for the application to finish
	done := make(chan struct{})

	// Register a JavaScript function to run our example
	js.Global().Set("runSQLiteExample", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		go func() {
			// Run the SQLite example
			if err := runExample(); err != nil {
				log.Printf("Error: %v", err)
			} else {
				log.Println("Example completed successfully!")
			}
			// Signal that we're done
			close(done)
		}()
		return nil
	}))

	// Wait for the application to finish
	<-done
}

func runExample() error {
	// Open a database with the IndexedDB VFS
	db, err := sql.Open("sqlite3", "file:mydatabase.db?vfs=idb")
	if err != nil {
		return err
	}
	defer db.Close()

	// Create a table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			name TEXT,
			email TEXT
		)
	`)
	if err != nil {
		return err
	}

	// Insert some data
	_, err = db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", "John Doe", "john@example.com")
	if err != nil {
		return err
	}

	// Query the data
	rows, err := db.Query("SELECT id, name, email FROM users")
	if err != nil {
		return err
	}
	defer rows.Close()

	// Print the results
	log.Println("Users in database:")
	for rows.Next() {
		var id int
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			return err
		}
		log.Printf("User %d: %s (%s)", id, name, email)
	}

	return rows.Err()
}
