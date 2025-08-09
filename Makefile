# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet
GOLINT=golangci-lint

# Binary info
BINARY_NAME=weekly-report-cli
BINARY_UNIX=$(BINARY_NAME)_unix
BUILD_DIR=./build

# Version info
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD)

# Linker flags
LDFLAGS=-ldflags="-s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.Commit=$(COMMIT)'"

.PHONY: all build clean test coverage deps fmt lint vet check install run help

# Default target
all: clean deps fmt vet lint test build

# Build the binary
build:
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v

# Build for production (optimized)
build-prod:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) -v

# Build for multiple platforms
build-all: clean
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 -v
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 -v
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 -v
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 -v
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe -v

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with race detection
test-race:
	$(GOTEST) -race -v ./...

# Run tests with coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with coverage and display in terminal
coverage-text:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -func=coverage.out

# Install/update dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy
	$(GOMOD) verify

# Format code
fmt:
	$(GOFMT) -w .

# Format and simplify code
fmt-fix:
	$(GOFMT) -w -s .

# Run go vet
vet:
	$(GOVET) ./...

# Run linter (requires golangci-lint)
lint:
	@if command -v $(GOLINT) > /dev/null 2>&1; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Install linter
install-lint:
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run all checks (format, vet, lint, test)
check: fmt vet lint test

# Install binary to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) .

# Run the application with example arguments
run:
	./$(BINARY_NAME) --help

# Run with sample data (requires links.txt file)
run-example:
	@if [ -f "examples/links.txt" ]; then \
		cat examples/links.txt | ./$(BINARY_NAME) generate --since-days 7; \
	else \
		echo "Create examples/links.txt with GitHub issue URLs to test"; \
	fi

# Development server with file watching (requires entr)
dev:
	@if command -v entr > /dev/null 2>&1; then \
		find . -name "*.go" | entr -r make build run; \
	else \
		echo "entr not installed. Install with your package manager (e.g., brew install entr)"; \
	fi

# Generate mocks (if using mockery)
mocks:
	@if command -v mockery > /dev/null 2>&1; then \
		mockery --all --output ./mocks; \
	else \
		echo "mockery not installed. Install with: go install github.com/vektra/mockery/v2@latest"; \
	fi

# Benchmark tests
bench:
	$(GOTEST) -bench=. -benchmem ./...

# Security scan (requires gosec)
security:
	@if command -v gosec > /dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

# Vulnerability check
vuln:
	@if command -v govulncheck > /dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

# Generate documentation
docs:
	@if command -v godoc > /dev/null 2>&1; then \
		echo "Starting godoc server at http://localhost:6060"; \
		godoc -http=:6060; \
	else \
		echo "godoc not installed. Install with: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi

# Create release archive
release: build-all
	cd $(BUILD_DIR) && \
	for binary in $(BINARY_NAME)-*; do \
		if [[ $$binary == *.exe ]]; then \
			zip $${binary%.exe}.zip $$binary; \
		else \
			tar -czf $$binary.tar.gz $$binary; \
		fi; \
	done

# Docker build (if Dockerfile exists)
docker-build:
	@if [ -f "Dockerfile" ]; then \
		docker build -t $(BINARY_NAME):$(VERSION) .; \
	else \
		echo "Dockerfile not found"; \
	fi

# Show project info
info:
	@echo "Binary Name: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Commit: $(COMMIT)"
	@echo "Go Version: $(shell $(GOCMD) version)"

# Display help
help:
	@echo "Available targets:"
	@echo "  all          - Run clean, deps, fmt, vet, lint, test, build"
	@echo "  build        - Build the binary"
	@echo "  build-prod   - Build optimized production binary"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-race    - Run tests with race detection"
	@echo "  coverage     - Run tests with coverage report"
	@echo "  coverage-text- Run tests with coverage in terminal"
	@echo "  deps         - Install/update dependencies"
	@echo "  fmt          - Format code"
	@echo "  fmt-fix      - Format and simplify code"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run linter"
	@echo "  install-lint - Install golangci-lint"
	@echo "  check        - Run all checks (fmt, vet, lint, test)"
	@echo "  install      - Install binary to GOPATH/bin"
	@echo "  run          - Run the application"
	@echo "  run-example  - Run with sample data"
	@echo "  dev          - Development mode with file watching"
	@echo "  mocks        - Generate mocks"
	@echo "  bench        - Run benchmark tests"
	@echo "  security     - Run security scan"
	@echo "  vuln         - Check for vulnerabilities"
	@echo "  docs         - Start godoc server"
	@echo "  release      - Create release archives"
	@echo "  docker-build - Build Docker image"
	@echo "  info         - Show project information"
	@echo "  help         - Show this help message"