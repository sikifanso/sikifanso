.PHONY: build run clean snapshot release-dry-run lint test

# Build for your current platform
build:
	go build -o sikifanso ./cmd/sikifanso

# Build and run
run:
	go run ./cmd/sikifanso

# Remove build artifacts
clean:
	rm -f sikifanso
	rm sikifanso.log
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

# Run tests with race detector
test:
	go test ./... -race
