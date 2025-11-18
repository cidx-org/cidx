.PHONY: all build clean test fmt vet install run-example help

# Variables
BINARY_NAME=cidx
BUILD_DIR=bin
GO_FILES=$(shell find . -name '*.go' -type f)

# Default target
all: fmt vet test build

# Build the binary
build:
	@echo "Building ${BINARY_NAME}..."
	@mkdir -p ${BUILD_DIR}
	@go build -o ${BUILD_DIR}/${BINARY_NAME} ./cmd/cidx
	@echo "Build complete: ${BUILD_DIR}/${BINARY_NAME}"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf ${BUILD_DIR}
	@go clean
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Install binary to GOPATH/bin
install: build
	@echo "Installing ${BINARY_NAME}..."
	@go install ./cmd/cidx
	@echo "Installed to $(shell go env GOPATH)/bin/${BINARY_NAME}"

# Run example
run-example:
	@echo "Running example..."
	@${BUILD_DIR}/${BINARY_NAME} list

# Initialize a test config
init-config:
	@echo "Initializing test config..."
	@${BUILD_DIR}/${BINARY_NAME} init

# Validate example config
validate-example:
	@echo "Validating example config..."
	@${BUILD_DIR}/${BINARY_NAME} validate -c examples/cidx.toml

# Show info for a tool
info-trivy:
	@${BUILD_DIR}/${BINARY_NAME} info trivy

# Dry-run example
dry-run:
	@echo "Dry-run example pipeline..."
	@${BUILD_DIR}/${BINARY_NAME} run -c examples/cidx.toml --dry-run ci

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p ${BUILD_DIR}
	@GOOS=linux GOARCH=amd64 go build -o ${BUILD_DIR}/${BINARY_NAME}-linux-amd64 ./cmd/cidx
	@GOOS=darwin GOARCH=amd64 go build -o ${BUILD_DIR}/${BINARY_NAME}-darwin-amd64 ./cmd/cidx
	@GOOS=darwin GOARCH=arm64 go build -o ${BUILD_DIR}/${BINARY_NAME}-darwin-arm64 ./cmd/cidx
	@GOOS=windows GOARCH=amd64 go build -o ${BUILD_DIR}/${BINARY_NAME}-windows-amd64.exe ./cmd/cidx
	@echo "Multi-platform build complete"

# Development workflow
dev: fmt vet build
	@echo "Development build complete"

# Help
help:
	@echo "CIDX Makefile targets:"
	@echo "  all            - Format, vet, test, and build"
	@echo "  build          - Build the binary"
	@echo "  clean          - Remove build artifacts"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  fmt            - Format Go code"
	@echo "  vet            - Run go vet"
	@echo "  deps           - Install/update dependencies"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  run-example    - Run cidx list command"
	@echo "  init-config    - Initialize a test config"
	@echo "  validate-example - Validate example config"
	@echo "  info-trivy     - Show info for trivy preset"
	@echo "  dry-run        - Dry-run example pipeline"
	@echo "  build-all      - Build for multiple platforms"
	@echo "  dev            - Quick development build (fmt + vet + build)"
	@echo "  help           - Show this help"
