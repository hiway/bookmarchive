# Makefile for bookmarchive project

# Version information
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY := $(shell whoami)@$(shell hostname)

# Build flags
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -X main.builtBy=$(BUILT_BY)"
BUILD_FLAGS := -trimpath $(LDFLAGS)
CGO_DISABLED := CGO_ENABLED=0

# Target platforms
PLATFORMS := \
	freebsd/amd64 \
	freebsd/arm64 \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

# Binary name
BINARY_NAME := bookmarchive

# Output directory for builds
BUILD_DIR := build

.PHONY: test test-verbose test-coverage clean build run format lint mod-tidy \
        build-all release $(addprefix build-, $(subst /,-,$(PLATFORMS))) \
        clean-build version goreleaser-snapshot goreleaser-release goreleaser-check

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
	rm -f $(BINARY_NAME) coverage.out coverage.html

# Clean all build artifacts including cross-platform builds
clean-build: clean
	rm -rf $(BUILD_DIR)

# Build the application for current platform
build:
	$(CGO_DISABLED) go build $(BUILD_FLAGS) -o $(BINARY_NAME) .

# Build with debug information (for development)
build-debug:
	go build -gcflags="all=-N -l" -o $(BINARY_NAME) .

# Show version information that would be embedded
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Date: $(DATE)"
	@echo "Built by: $(BUILT_BY)"

# Generate build targets for each platform
define build-platform
build-$(subst /,-,$(1)): 
	@echo "Building for $(1)..."
	@mkdir -p $(BUILD_DIR)
	$(CGO_DISABLED) GOOS=$(word 1,$(subst /, ,$(1))) GOARCH=$(word 2,$(subst /, ,$(1))) \
	go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-$(subst /,-,$(1))$(if $(findstring windows,$(1)),.exe) .
endef

# Create build targets for all platforms
$(foreach platform,$(PLATFORMS),$(eval $(call build-platform,$(platform))))

# Build for all platforms
build-all: $(addprefix build-, $(subst /,-,$(PLATFORMS)))
	@echo "All builds completed in $(BUILD_DIR)/"

# Release build (same as build-all but with clear messaging)
release: clean-build build-all
	@echo "Release builds completed for all platforms:"
	@ls -la $(BUILD_DIR)/

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

# GoReleaser targets
goreleaser-check:
	goreleaser check

goreleaser-snapshot:
	goreleaser build --snapshot --clean

goreleaser-release:
	goreleaser release --clean

# Install GoReleaser (for development)
install-goreleaser:
	go install github.com/goreleaser/goreleaser@latest
