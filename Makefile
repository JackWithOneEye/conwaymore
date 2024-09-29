all: build

build:
	@templ generate
	@bunx esbuild cmd/web/frontend/index.js cmd/web/frontend/vendor.js cmd/web/frontend/worker/worker.js  --bundle --outdir=cmd/web/assets/js --format=esm
	@bunx tailwindcss -i cmd/web/frontend/input.css -o cmd/web/assets/css/output.css
	@go build -o main cmd/api/main.go

clean:
	@echo "Cleaning..."
	@rm -f main