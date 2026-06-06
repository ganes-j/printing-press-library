.PHONY: build test lint install clean

build:
	go build -o bin/pypi-pp-cli ./cmd/pypi-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pypi-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pypi-pp-mcp ./cmd/pypi-pp-mcp

install-mcp:
	go install ./cmd/pypi-pp-mcp

build-all: build build-mcp
