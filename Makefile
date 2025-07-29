# Makefile for bookmarchive project

.PHONY: test test-verbose test-coverage clean build run format lint mod-tidy

# Default target
all: build

# Run tests
test:
	go test ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	go clean
	rm -f bookmarchive coverage.out coverage.html

# Build the application
build:
	go build -o bookmarchive .

# Run the application
run:
	go run .

# Format code
format:
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Tidy module dependencies
mod-tidy:
	go mod tidy
