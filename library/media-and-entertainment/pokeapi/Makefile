.PHONY: build test lint install clean

build:
	go build -o bin/pokeapi-pp-cli ./cmd/pokeapi-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/pokeapi-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/pokeapi-pp-mcp ./cmd/pokeapi-pp-mcp

install-mcp:
	go install ./cmd/pokeapi-pp-mcp

build-all: build build-mcp
