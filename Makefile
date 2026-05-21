.PHONY: build test lint install clean

build:
	go build -o bin/lawmatics-pp-cli ./cmd/lawmatics-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/lawmatics-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/lawmatics-pp-mcp ./cmd/lawmatics-pp-mcp

install-mcp:
	go install ./cmd/lawmatics-pp-mcp

build-all: build build-mcp
