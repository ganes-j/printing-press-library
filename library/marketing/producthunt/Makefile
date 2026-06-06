.PHONY: build test lint install clean

build:
	go build -o bin/producthunt-pp-cli ./cmd/producthunt-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/producthunt-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/producthunt-pp-mcp ./cmd/producthunt-pp-mcp

install-mcp:
	go install ./cmd/producthunt-pp-mcp

build-all: build build-mcp
