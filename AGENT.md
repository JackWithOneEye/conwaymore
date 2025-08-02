# ConwayMore Agent Guidelines

## Build/Test Commands
- `make build` - Full build (templ generate, frontend build, WASM compile, tailwind, main binary)
- `make test-integration` - Run integration tests
- `go test -v ./tests/integration/...` - Run all integration tests
- `go test -v ./tests/integration/api -run TestAPISuite/TestHealthCheck` - Run single test
- `make terminal` - Run terminal client
- `go run cmd/api/main.go` - Run API server
- `bunx tailwindcss -i cmd/web/frontend/input.css -o cmd/web/assets/css/output.css` - Build CSS
- **Use bun instead of npm** for package management and running commands

## Architecture
- **Conway's Game of Life implementation** with real-time WebSocket communication
- **cmd/**: Entry points - api (server), terminal (TUI client), wasm (browser client), web (frontend), build (bundler)
- **internal/**: Core modules - engine (game logic), server (HTTP/WS), conway (game rules), database (SQLite), tui (terminal UI), patterns (game patterns), protocol (message format)
- **Database**: SQLite for persistence (seed storage)
- **Frontend**: HTMX + Templ templates + WASM + Tailwind CSS

## Code Style
- **Imports**: Grouped (stdlib, external, internal) with `github.com/JackWithOneEye/conwaymore/internal/` prefix
- **Error handling**: Return explicit errors, use `log.Fatalf` for startup failures
- **Interfaces**: Small, focused (e.g., `ServerConfig`, `Engine`, `DatabaseService`)
- **Naming**: CamelCase for public, lowercase for private, descriptive function names
- **Testing**: Use `testify/suite` pattern with `SetupTest`/`TearDownTest` lifecycle
- **Context**: Pass `context.Context` for cancellation/timeouts
