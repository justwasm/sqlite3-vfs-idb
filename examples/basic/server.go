//go:build !js || !wasm

package main

import (
	"log"
	"net/http"
)

func main() {
	// Serve static files from the current directory
	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)

	// Start the server
	log.Println("Starting server on http://localhost:8080")
	log.Println("Press Ctrl+C to stop")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
