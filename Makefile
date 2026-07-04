# Local developer build. GoReleaser owns release builds (see .goreleaser.yaml);
# this just gives `make build` a versioned bin/spur for hand testing. Version is
# derived from git so a local binary reports something better than "dev".
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_STAMP := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/spurintel/cli/internal/buildinfo.Version=$(VERSION) \
	-X github.com/spurintel/cli/internal/buildinfo.BuildStamp=$(BUILD_STAMP)

.PHONY: all fmt vet test build

all: fmt vet test build

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/spur ./cmd/spur
