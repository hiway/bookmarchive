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
        clean-build version goreleaser-snapshot goreleaser-release goreleaser-check \
        tag-patch tag-minor tag-major push-tag check-clean pre-release \
        show-next-versions release-patch release-minor release-major help

# Default target
all: build

# Show help information
help:
	@echo "BookmArchive Build System"
	@echo "========================="
	@echo ""
	@echo "Development targets:"
	@echo "  build              Build the application for current platform"
	@echo "  build-debug        Build with debug information"
	@echo "  run                Run the application"
	@echo "  test               Run tests"
	@echo "  test-verbose       Run tests with verbose output"
	@echo "  test-coverage      Run tests with coverage report"
	@echo "  format             Format code"
	@echo "  lint               Run linter"
	@echo "  mod-tidy           Tidy module dependencies"
	@echo ""
	@echo "Cross-platform build targets:"
	@echo "  build-all          Build for all supported platforms"
	@echo "  build-linux-amd64  Build for Linux AMD64"
	@echo "  build-darwin-arm64 Build for macOS ARM64"
	@echo "  build-windows-amd64 Build for Windows AMD64"
	@echo "  (and more platform combinations...)"
	@echo ""
	@echo "Version management:"
	@echo "  show-next-versions Show next possible version numbers"
	@echo "  check-clean        Check if working directory is clean"
	@echo "  pre-release        Run all pre-release checks"
	@echo "  tag-patch          Create and tag a patch release (x.y.Z)"
	@echo "  tag-minor          Create and tag a minor release (x.Y.0)"
	@echo "  tag-major          Create and tag a major release (X.0.0)"
	@echo "  push-tag           Push the latest tag to origin"
	@echo ""
	@echo "Complete release workflows:"
	@echo "  release-patch      Tag patch version and push to origin"
	@echo "  release-minor      Tag minor version and push to origin"
	@echo "  release-major      Tag major version and push to origin"
	@echo ""
	@echo "GoReleaser targets:"
	@echo "  goreleaser-check   Check GoReleaser configuration"
	@echo "  goreleaser-snapshot Create snapshot build"
	@echo "  goreleaser-release Create official release"
	@echo ""
	@echo "Utility targets:"
	@echo "  version            Show current version information"
	@echo "  clean              Clean build artifacts"
	@echo "  clean-build        Clean all build artifacts"
	@echo "  help               Show this help message"

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

# =============================================================================
# VERSION MANAGEMENT AND RELEASE TARGETS
# =============================================================================

# Check if working directory is clean
check-clean:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory is not clean. Please commit or stash changes."; \
		echo "Uncommitted changes:"; \
		git status --short; \
		exit 1; \
	fi
	@echo "✓ Working directory is clean"

# Get the latest tag
LATEST_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# Extract version components from latest tag
MAJOR := $(shell echo $(LATEST_TAG) | sed 's/v//' | cut -d. -f1)
MINOR := $(shell echo $(LATEST_TAG) | sed 's/v//' | cut -d. -f2)
PATCH := $(shell echo $(LATEST_TAG) | sed 's/v//' | cut -d. -f3)

# Calculate next version numbers
NEXT_PATCH := v$(MAJOR).$(MINOR).$(shell echo $$(($(PATCH) + 1)))
NEXT_MINOR := v$(MAJOR).$(shell echo $$(($(MINOR) + 1))).0
NEXT_MAJOR := v$(shell echo $$(($(MAJOR) + 1))).0.0

# Pre-release checks (tests and linting)
pre-release: check-clean
	@echo "Running pre-release checks..."
	@make test
	@make lint
	@make mod-tidy
	@echo "✓ All pre-release checks passed"

# Create a patch version tag (e.g., v1.2.3 -> v1.2.4)
tag-patch: pre-release
	@echo "Current version: $(LATEST_TAG)"
	@echo "Next patch version: $(NEXT_PATCH)"
	@read -p "Create patch tag $(NEXT_PATCH)? [y/N] " confirm && [ "$$confirm" = "y" ]
	git tag -a $(NEXT_PATCH) -m "Release $(NEXT_PATCH)"
	@echo "✓ Created tag $(NEXT_PATCH)"
	@echo "Run 'make push-tag' to push the tag to origin"

# Create a minor version tag (e.g., v1.2.3 -> v1.3.0)
tag-minor: pre-release
	@echo "Current version: $(LATEST_TAG)"
	@echo "Next minor version: $(NEXT_MINOR)"
	@read -p "Create minor tag $(NEXT_MINOR)? [y/N] " confirm && [ "$$confirm" = "y" ]
	git tag -a $(NEXT_MINOR) -m "Release $(NEXT_MINOR)"
	@echo "✓ Created tag $(NEXT_MINOR)"
	@echo "Run 'make push-tag' to push the tag to origin"

# Create a major version tag (e.g., v1.2.3 -> v2.0.0)
tag-major: pre-release
	@echo "Current version: $(LATEST_TAG)"
	@echo "Next major version: $(NEXT_MAJOR)"
	@read -p "Create major tag $(NEXT_MAJOR)? [y/N] " confirm && [ "$$confirm" = "y" ]
	git tag -a $(NEXT_MAJOR) -m "Release $(NEXT_MAJOR)"
	@echo "✓ Created tag $(NEXT_MAJOR)"
	@echo "Run 'make push-tag' to push the tag to origin"

# Push the latest tag to origin
push-tag:
	@LATEST_LOCAL_TAG=$$(git describe --tags --abbrev=0 2>/dev/null); \
	if [ -z "$$LATEST_LOCAL_TAG" ]; then \
		echo "Error: No tags found"; \
		exit 1; \
	fi; \
	echo "Pushing tag $$LATEST_LOCAL_TAG to origin..."; \
	git push origin $$LATEST_LOCAL_TAG
	@echo "✓ Tag pushed to origin"
	@echo "GitHub Actions should automatically create a release"

# Show next possible version numbers
show-next-versions:
	@echo "Current version: $(LATEST_TAG)"
	@echo "Next patch version: $(NEXT_PATCH)"
	@echo "Next minor version: $(NEXT_MINOR)"
	@echo "Next major version: $(NEXT_MAJOR)"

# Complete release workflow: tag and push
release-patch: tag-patch push-tag
	@echo "✓ Patch release completed"

release-minor: tag-minor push-tag
	@echo "✓ Minor release completed"

release-major: tag-major push-tag
	@echo "✓ Major release completed"
