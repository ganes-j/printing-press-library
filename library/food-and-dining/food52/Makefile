.PHONY: build test lint install clean

build:
	go build -o bin/food52-pp-cli ./cmd/food52-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/food52-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/food52-pp-mcp ./cmd/food52-pp-mcp

install-mcp:
	go install ./cmd/food52-pp-mcp

build-all: build build-mcp
