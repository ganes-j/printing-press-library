.PHONY: build test lint install clean

build:
	go build -o bin/amazon-seller-pp-cli ./cmd/amazon-seller-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/amazon-seller-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/amazon-seller-pp-mcp ./cmd/amazon-seller-pp-mcp

install-mcp:
	go install ./cmd/amazon-seller-pp-mcp

build-all: build build-mcp
