.PHONY: build test lint install clean

build:
	go build -o bin/allrecipes-pp-cli ./cmd/allrecipes-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/allrecipes-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/allrecipes-pp-mcp ./cmd/allrecipes-pp-mcp

install-mcp:
	go install ./cmd/allrecipes-pp-mcp

build-all: build build-mcp
