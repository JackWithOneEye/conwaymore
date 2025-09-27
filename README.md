# ConwayMore

A real-time Conway's Game of Life implementation with WebSocket communication, featuring multiple clients (web, terminal, WASM) and persistent game state storage.

## Quick Start

### Prerequisites

- Go 1.25+
- Bun (for frontend dependencies)
- air (for development)

### Build & Run

```bash
# Install dependencies and build everything
make build

# Run the API server
go run cmd/api/main.go

# In another terminal, run the terminal client
make terminal
```

### Development Commands

- `air` - Build and run the application with hot reload
- `make terminal` - Build and run the terminal client
- `make test-integration` - Run integration tests

Visit `http://localhost:8080` for the web interface.

