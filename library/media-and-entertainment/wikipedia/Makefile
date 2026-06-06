.PHONY: build test lint install clean

build:
	go build -o bin/wikipedia-pp-cli ./cmd/wikipedia-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/wikipedia-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/wikipedia-pp-mcp ./cmd/wikipedia-pp-mcp

install-mcp:
	go install ./cmd/wikipedia-pp-mcp

build-all: build build-mcp
