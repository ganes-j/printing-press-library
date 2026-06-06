.PHONY: build test lint install clean

build:
	go build -o bin/nvd-pp-cli ./cmd/nvd-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/nvd-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/nvd-pp-mcp ./cmd/nvd-pp-mcp

install-mcp:
	go install ./cmd/nvd-pp-mcp

build-all: build build-mcp
