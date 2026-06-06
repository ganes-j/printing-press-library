.PHONY: build test lint install clean

build:
	go build -o bin/open-meteo-pp-cli ./cmd/open-meteo-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/open-meteo-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/open-meteo-pp-mcp ./cmd/open-meteo-pp-mcp

install-mcp:
	go install ./cmd/open-meteo-pp-mcp

build-all: build build-mcp
