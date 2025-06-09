# Conway's Game of Life (ConwayMore)

## Build/Test Commands
- `make build` - Full build (templ generate, frontend, WASM, CSS, Go binary)
- `make test-integration` - Run integration tests
- `go test -v ./tests/integration/...` - Run all integration tests
- `go test -v ./tests/integration/api/` - Run API tests specifically
- `air` - Hot reload development server (uses .air.toml config)
- `bun install` - Install JavaScript dependencies

## Architecture
- **Web server**: Gin-based HTTP/WebSocket API in `cmd/api/`
- **Conway engine**: Real-time Conway's Game of Life simulator in `internal/engine/`
- **Database**: SQLite for persistence via `internal/database/`
- **Frontend**: HTMX + Tailwind CSS + Go WASM in `cmd/web/`
- **Build system**: Custom frontend builder in `cmd/build/`

## Code Style
- **Package structure**: `internal/` for private code, `cmd/` for executables
- **Error handling**: Standard Go patterns with explicit error returns
- **Imports**: Group stdlib, third-party, internal packages separately
- **Naming**: PascalCase for exported, camelCase for unexported
- **Interfaces**: Defined in consumer packages (e.g., `EngineConfig` in engine)
- **Testing**: Use testify/suite for integration tests
