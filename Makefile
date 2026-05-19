VERSION := $(shell cat VERSION 2>/dev/null || echo "0.1.0-dev")
BINARY := dsn
GOFLAGS := -ldflags="-X main.Version=$(VERSION)"

.PHONY: all build test test-integration lint clean docker-build build-all version

all: lint test build

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)
	go build $(GOFLAGS) -o bin/$(BINARY)-tui ./cmd/$(BINARY)-tui

test:
	go test ./... -cover -count=1 -timeout=120s

test-integration:
	go test -tags=integration -count=1 -timeout=300s ./integration/...

lint:
	go vet ./...
	@echo "Checking for staticcheck..."
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

clean:
	rm -rf bin/ dist/

docker-build:
	docker build -t ghcr.io/dsn/$(BINARY):$(VERSION) -f dev/localnet/Dockerfile .

build-all:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-amd64 ./cmd/$(BINARY)
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-arm64 ./cmd/$(BINARY)
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-darwin-amd64 ./cmd/$(BINARY)
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-windows-amd64.exe ./cmd/$(BINARY)
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-tui-linux-amd64 ./cmd/$(BINARY)-tui

version:
	@echo $(VERSION)