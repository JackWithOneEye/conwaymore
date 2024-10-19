all: build

build:
	@templ generate
	@if [ ! -e ./bin/build_frontend ]; then \
		go build -o ./bin/build_frontend cmd/build/main.go; \
	fi	
	@./bin/build_frontend
	@GOOS=js GOARCH=wasm go build -o cmd/web/assets/js/go.wasm cmd/wasm/main.go
	@bunx tailwindcss -i cmd/web/frontend/input.css -o cmd/web/assets/css/output.css
	@go build -o ./bin/main cmd/api/main.go

clean:
	@echo "Cleaning..."
	@rm -f main
