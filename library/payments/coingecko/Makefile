.PHONY: build test lint install clean

build:
	go build -o bin/coingecko-pp-cli ./cmd/coingecko-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/coingecko-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/coingecko-pp-mcp ./cmd/coingecko-pp-mcp

install-mcp:
	go install ./cmd/coingecko-pp-mcp

build-all: build build-mcp
