.PHONY: build run clean

# Default target
all: build

# Build the WebAssembly binary
build:
	@echo "Building WebAssembly binary..."
	GOOS=js GOARCH=wasm go build -o main.wasm main.go
	@echo "Copying WebAssembly support file..."
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" .
	@echo "Build complete. Run 'make run' to start the server."

# Run the server
run: build
	@echo "Starting server on http://localhost:8080"
	go run server.go

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f main.wasm wasm_exec.js
	@echo "Clean complete."

# Help
help:
	@echo "Available targets:"
	@echo "  make build  - Build the WebAssembly binary"
	@echo "  make run    - Build and run the server"
	@echo "  make clean  - Remove build artifacts"
	@echo "  make help   - Show this help message"