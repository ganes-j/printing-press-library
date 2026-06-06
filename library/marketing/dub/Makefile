.PHONY: build test lint install clean

build:
	go build -o bin/dub-pp-cli ./cmd/dub-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/dub-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/dub-pp-mcp ./cmd/dub-pp-mcp

install-mcp:
	go install ./cmd/dub-pp-mcp

build-all: build build-mcp
