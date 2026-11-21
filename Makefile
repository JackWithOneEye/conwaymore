all: build

build:
	@templ generate
	@go build -o ./bin/build_frontend cmd/build/main.go
	@./bin/build_frontend
	@GOOS=js GOARCH=wasm go build -o internal/web/assets/js/go.wasm cmd/wasm/main.go
	@GOOS=js GOARCH=wasm go build -o internal/web/assets/js/engine.wasm ./cmd/wasmengine
	@bunx tailwindcss -i internal/web/frontend/input.css -o internal/web/assets/css/output.css
	@go build -o ./bin/main cmd/api/main.go

clean:
	@echo "Cleaning..."
	@rm -f main

.PHONY: test-integration terminal
test-integration:
	go test -v ./tests/integration/...

bench-conway:
	go test -v -bench=. -count=10 -benchmem ./tests/benchmarks/conway/...

terminal:
	go run cmd/terminal/main.go
