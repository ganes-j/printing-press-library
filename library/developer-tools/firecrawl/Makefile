.PHONY: build test lint install clean

build:
	go build -o bin/firecrawl-pp-cli ./cmd/firecrawl-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/firecrawl-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/firecrawl-pp-mcp ./cmd/firecrawl-pp-mcp

install-mcp:
	go install ./cmd/firecrawl-pp-mcp

build-all: build build-mcp
