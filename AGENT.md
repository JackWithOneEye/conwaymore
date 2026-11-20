# ConwayMore Agent Guidelines

## Build/Test Commands
- `make build` - Full build (templ generate, frontend build, WASM compile, tailwind, main binary)
- `make test-integration` - Run integration tests
- `go test -v ./tests/integration/...` - Run all integration tests
- `go test -v ./tests/integration/wire/...` - Run wire-efficiency tests (frame-ack flow control, speed-change bandwidth); look for RESULT lines
- `go test -v ./tests/integration/api -run TestAPISuite/TestHealthCheck` - Run single test
- `PATH="$(go env GOROOT)/lib/wasm:$PATH" GOOS=js GOARCH=wasm go test ./cmd/wasmengine/...` - Run the engine worker tests in WASM under Node, the browser workers' single-threaded runtime (`*_js_test.go` only run there)
- `make terminal` - Run terminal client
- `go run cmd/api/main.go` - Run API server
- `bunx tailwindcss -i cmd/web/frontend/input.css -o cmd/web/assets/css/output.css` - Build CSS
- **Use bun instead of npm** for package management and running commands

## Architecture
- **Conway's Game of Life implementation** with real-time WebSocket communication
- **cmd/**: Entry points - api (server), terminal (TUI client), wasm (browser canvas worker), wasmengine (browser engine worker for single-player games), web (frontend), build (bundler)
- **internal/**: Core modules - engine (game logic), server (HTTP/WS), conway (game rules), database (SQLite), tui (terminal UI), patterns (game patterns), protocol (message format)
- **Database**: SQLite for persistence (seed storage)
- **Frontend**: HTMX + Templ templates + WASM + Tailwind CSS

## Game Modes
- **Multiplayer** (`mode = 'multi'`): the server runs the engine (`internal/games` rooms) and streams frames over `/games/:id/play`; `POST /api/games/:id/save` stores its state
- **Single-player** (`mode = 'single'`): the browser's engine worker (`cmd/wasmengine`) runs the same `internal/engine` on its own thread; the server never loads it, it only stores its state via `GET`/`PUT /api/games/:id/seed` (an encoded `protocol.Output`, validated against the world size). The page starts the engine worker from the stored state, asks it for snapshots to save, and connects it to the canvas worker with a `MessageChannel`
- `cmd/wasm/session` hides the difference from the canvas worker: `Remote` (WebSocket to the server) and `Worker` (MessagePort to the engine worker) both implement `Session`, with the same wire protocol and frame acks

## Wire Protocol & Flow Control
- **Client -> server**: `Command`, `SetCells`, `SetSpeed`, `FrameAck` (see `internal/protocol/client_message.go`)
- **Server -> client**: `Output` (full frame) and `StateUpdate` (0xff marker + playing + speed, 4 bytes, for changes that leave cells untouched, e.g. speed changes)
- **Frame acks**: every client acks every received message. The server and the engine worker keep at most `protocol.MaxInFlight` (2) unacked messages per client; while delivery is in progress, the pending message is *replaced* by the newest one (state updates are folded into a pending frame's header via `protocol.ApplyStateUpdate`)
- **Speed slider** (web): debounced (`SPEED_SEND_DEBOUNCE_MS` in `cmd/web/frontend/index.js`) so drags don't flood commands

## Code Style
- **Imports**: Grouped (stdlib, external, internal) with `github.com/JackWithOneEye/conwaymore/internal/` prefix
- **Error handling**: Return explicit errors, use `log.Fatalf` for startup failures
- **Interfaces**: Small, focused (e.g., `ServerConfig`, `Engine`, `DatabaseService`)
- **Naming**: CamelCase for public, lowercase for private, descriptive function names
- **Testing**: Use `testify/suite` pattern with `SetupTest`/`TearDownTest` lifecycle
- **Context**: Pass `context.Context` for cancellation/timeouts
