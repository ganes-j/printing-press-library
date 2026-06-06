.PHONY: build test lint install clean

build:
	go build -o bin/company-goat-pp-cli ./cmd/company-goat-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/company-goat-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/company-goat-pp-mcp ./cmd/company-goat-pp-mcp

install-mcp:
	go install ./cmd/company-goat-pp-mcp

build-all: build build-mcp
