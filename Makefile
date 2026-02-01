BINARY_NAME=hatch
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/EscapeVelocityOperations/hatch-cli/cmd/root.version=$(VERSION) -X github.com/EscapeVelocityOperations/hatch-cli/cmd/root.commit=$(COMMIT) -X github.com/EscapeVelocityOperations/hatch-cli/cmd/root.date=$(DATE)"

.PHONY: build install test lint release release-snapshot clean

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/hatch

install:
	go install $(LDFLAGS) ./cmd/hatch

test:
	go test ./...

lint:
	golangci-lint run

release:
	goreleaser release --clean

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin/ dist/
