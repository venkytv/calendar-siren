# Meeting Siren Makefile
# Build system for cross-platform binaries

# Build variables
BINARY_NAME=meeting-siren
MAIN_PATH=cmd/meeting-siren/main.go
BUILD_DIR=build

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Default target
.PHONY: all
all: clean build-macos build-linux-arm64

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Build for macOS (Apple Silicon)
.PHONY: build-macos
build-macos: $(BUILD_DIR)
	@echo "Building for macOS (arm64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

# Build for Raspberry Pi (Linux ARM64)
.PHONY: build-linux-arm64
build-linux-arm64: $(BUILD_DIR)
	@echo "Building for Linux (arm64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)

# Build for current platform (development)
.PHONY: build-dev
build-dev: $(BUILD_DIR)
	@echo "Building for current platform..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test ./...

# Run tests with coverage
.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	go test -cover ./...

# Run integration tests (requires NATS server)
.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	go test -tags=integration ./test/integration/

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

# Run linter (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Code quality checks
.PHONY: check
check: fmt vet lint test

# Development run
.PHONY: run
run:
	go run $(MAIN_PATH)

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Show build information
.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all              - Build binaries for macOS and Raspberry Pi"
	@echo "  build-dev        - Build for current platform"
	@echo "  build-macos      - Build for macOS (arm64)"
	@echo "  build-linux-arm64 - Build for Raspberry Pi (arm64)"
	@echo "  clean            - Remove build artifacts"
	@echo "  test             - Run unit tests"
	@echo "  test-cover       - Run tests with coverage"
	@echo "  test-integration - Run integration tests"
	@echo "  fmt              - Format code"
	@echo "  lint             - Run linter"
	@echo "  vet              - Run go vet"
	@echo "  check            - Run all code quality checks"
	@echo "  run              - Run the application"
	@echo "  deps             - Install dependencies"
	@echo "  version          - Show build information"
	@echo "  help             - Show this help message"