APP_NAME := sikifanso

.PHONY: build run clean snapshot release-dry-run lint test test-integration vet fmt generate install docs-setup docs docs-serve

# Build for your current platform
build:
	go build -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

# Build and run
run:
	go run ./cmd/$(APP_NAME)

# Install to $GOPATH/bin
install:
	go install ./cmd/$(APP_NAME)

# Remove build artifacts
clean:
	rm -rf bin/
	rm -f sikifanso.log
	rm -rf dist/

# GoReleaser: build snapshot (cross-platform, no publish)
snapshot:
	goreleaser build --snapshot --clean

# GoReleaser: full dry run (archives + checksums, no publish)
release-dry-run:
	goreleaser release --snapshot --clean

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Format code with goimports
fmt:
	goimports -w .

# Run code generation
generate:
	go generate ./...

# Run tests with race detector
test:
	go test -race -count=1 ./...

# Run integration tests
test-integration:
	go test -race -count=1 -tags=integration ./...

# Install docs toolchain (one-time setup)
docs-setup:
	python3 -m venv .venv
	.venv/bin/pip install zensical

# Build documentation site (output: site/)
# Requires: make docs-setup (zensical is an MkDocs-compatible static site generator)
docs:
	.venv/bin/zensical build

# Serve docs locally with live reload (http://127.0.0.1:8000)
docs-serve:
	.venv/bin/zensical serve
