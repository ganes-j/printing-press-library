.PHONY: build test lint install clean

build:
	go build -o bin/docker-hub-pp-cli ./cmd/docker-hub-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/docker-hub-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/docker-hub-pp-mcp ./cmd/docker-hub-pp-mcp

install-mcp:
	go install ./cmd/docker-hub-pp-mcp

build-all: build build-mcp
