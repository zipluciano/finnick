VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build web test snapshot dev-api dev-web clean

build: web ## Build the UI and the binary (bin/finnick)
	go build -ldflags "$(LDFLAGS)" -o bin/finnick ./cmd/finnick

web: web/node_modules
	npm --prefix web run build

web/node_modules: web/package.json
	npm --prefix web install
	@touch $@

test:
	go test ./...
	npm --prefix web run typecheck

snapshot: ## Release archives for every platform in dist/, without publishing (needs GoReleaser)
	goreleaser release --snapshot --clean --skip=publish

dev-api: ## API on :7777 for the Vite dev server (pass DIR=...)
	go run ./cmd/finnick ui --port 7777 --no-open --dir $(or $(DIR),.)

dev-web: web/node_modules ## Vite on :5173, proxying /api to :7777
	npm --prefix web run dev

clean:
	rm -rf bin dist web/dist/assets web/dist/index.html web/dist/favicon.svg
