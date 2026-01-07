# PacketPony Makefile

# Variables
BINARY_NAME=packetpony
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"
BUILD_DIR=build
MAIN_PATH=./cmd/packetpony

# Installation paths
PREFIX?=/usr/local
BINDIR=$(PREFIX)/bin
SYSCONFDIR?=/etc
CONFIGDIR=$(SYSCONFDIR)/packetpony
SYSTEMDDIR=/etc/systemd/system

# Colors for output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

.PHONY: all build clean test coverage lint fmt vet run install uninstall help
.PHONY: release cross-compile docker deps update-deps
.PHONY: install-service uninstall-service

# Default target
all: clean fmt vet test build

## help: Display this help message
help:
	@echo "$(COLOR_BOLD)PacketPony Makefile$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Usage:$(COLOR_RESET)"
	@echo "  make [target]"
	@echo ""
	@echo "$(COLOR_BOLD)Available targets:$(COLOR_RESET)"
	@grep -E '^## ' Makefile | sed 's/^## /  $(COLOR_GREEN)/' | sed 's/:/ $(COLOR_RESET)-/'
	@echo ""

## build: Build the binary
build:
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME)...$(COLOR_RESET)"
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)Build complete: $(BINARY_NAME)$(COLOR_RESET)"

## build-debug: Build with debug symbols
build-debug:
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME) with debug symbols...$(COLOR_RESET)"
	$(GOBUILD) -gcflags="all=-N -l" -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)Debug build complete: $(BINARY_NAME)$(COLOR_RESET)"

## release: Build optimized release binary
release:
	@echo "$(COLOR_BLUE)Building release version $(VERSION)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)Release build complete: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

## cross-compile: Build for multiple platforms
cross-compile:
	@echo "$(COLOR_BLUE)Cross-compiling for multiple platforms...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-freebsd-amd64 $(MAIN_PATH)
	@echo "$(COLOR_GREEN)Cross-compilation complete$(COLOR_RESET)"
	@ls -lh $(BUILD_DIR)/

## clean: Remove build artifacts
clean:
	@echo "$(COLOR_YELLOW)Cleaning...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	@echo "$(COLOR_GREEN)Clean complete$(COLOR_RESET)"

## test: Run tests
test:
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	$(GOTEST) -v -race ./...

## test-short: Run tests without race detection
test-short:
	@echo "$(COLOR_BLUE)Running tests (short)...$(COLOR_RESET)"
	$(GOTEST) -v -short ./...

## coverage: Generate test coverage report
coverage:
	@echo "$(COLOR_BLUE)Generating coverage report...$(COLOR_RESET)"
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(COLOR_GREEN)Coverage report generated: coverage.html$(COLOR_RESET)"

## bench: Run benchmarks
bench:
	@echo "$(COLOR_BLUE)Running benchmarks...$(COLOR_RESET)"
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run linters (requires golangci-lint)
lint:
	@echo "$(COLOR_BLUE)Running linters...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Install it from https://golangci-lint.run/$(COLOR_RESET)" && exit 1)
	golangci-lint run --timeout=5m

## fmt: Format code
fmt:
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	$(GOFMT) -s -w .
	@echo "$(COLOR_GREEN)Format complete$(COLOR_RESET)"

## fmt-check: Check if code is formatted
fmt-check:
	@echo "$(COLOR_BLUE)Checking code format...$(COLOR_RESET)"
	@test -z "$$($(GOFMT) -s -l . | tee /dev/stderr)" || (echo "$(COLOR_YELLOW)Code is not formatted. Run 'make fmt'$(COLOR_RESET)" && exit 1)
	@echo "$(COLOR_GREEN)Code is properly formatted$(COLOR_RESET)"

## vet: Run go vet
vet:
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	$(GOVET) ./...
	@echo "$(COLOR_GREEN)Vet complete$(COLOR_RESET)"

## deps: Download dependencies
deps:
	@echo "$(COLOR_BLUE)Downloading dependencies...$(COLOR_RESET)"
	$(GOMOD) download
	@echo "$(COLOR_GREEN)Dependencies downloaded$(COLOR_RESET)"

## update-deps: Update dependencies
update-deps:
	@echo "$(COLOR_BLUE)Updating dependencies...$(COLOR_RESET)"
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Dependencies updated$(COLOR_RESET)"

## tidy: Tidy go.mod
tidy:
	@echo "$(COLOR_BLUE)Tidying go.mod...$(COLOR_RESET)"
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Tidy complete$(COLOR_RESET)"

## run: Build and run with example config
run: build
	@echo "$(COLOR_BLUE)Running $(BINARY_NAME)...$(COLOR_RESET)"
	./$(BINARY_NAME) -config configs/example.yaml

## run-test: Build and run with test config
run-test: build
	@echo "$(COLOR_BLUE)Running $(BINARY_NAME) with test config...$(COLOR_RESET)"
	./$(BINARY_NAME) -config configs/test.yaml

## install: Install binary and config
install: build
	@echo "$(COLOR_BLUE)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY_NAME) $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	@if [ ! -f $(DESTDIR)$(CONFIGDIR)/config.yaml ]; then \
		install -d $(DESTDIR)$(CONFIGDIR); \
		install -m 644 configs/example.yaml $(DESTDIR)$(CONFIGDIR)/config.yaml; \
		echo "$(COLOR_GREEN)Config installed to $(CONFIGDIR)/config.yaml$(COLOR_RESET)"; \
	else \
		echo "$(COLOR_YELLOW)Config already exists at $(CONFIGDIR)/config.yaml (not overwriting)$(COLOR_RESET)"; \
	fi
	@echo "$(COLOR_GREEN)Installation complete$(COLOR_RESET)"

## uninstall: Uninstall binary
uninstall:
	@echo "$(COLOR_YELLOW)Uninstalling $(BINARY_NAME)...$(COLOR_RESET)"
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	@echo "$(COLOR_YELLOW)Note: Config files in $(CONFIGDIR) were not removed$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)Uninstall complete$(COLOR_RESET)"

## install-service: Install systemd service (requires root)
install-service: install
	@echo "$(COLOR_BLUE)Installing systemd service...$(COLOR_RESET)"
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "$(COLOR_YELLOW)This target requires root privileges. Run with sudo.$(COLOR_RESET)"; \
		exit 1; \
	fi
	@if ! id packetpony >/dev/null 2>&1; then \
		useradd -r -s /bin/false -d /var/lib/packetpony packetpony; \
		echo "$(COLOR_GREEN)Created user 'packetpony'$(COLOR_RESET)"; \
	fi
	install -d /var/lib/packetpony
	install -d /var/log/packetpony
	chown packetpony:packetpony /var/lib/packetpony
	chown packetpony:packetpony /var/log/packetpony
	chown root:packetpony $(CONFIGDIR)/config.yaml
	chmod 640 $(CONFIGDIR)/config.yaml
	install -m 644 deployment/systemd/packetpony.service $(SYSTEMDDIR)/packetpony.service
	systemctl daemon-reload
	@echo "$(COLOR_GREEN)Service installed. Enable with: systemctl enable packetpony$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)Start with: systemctl start packetpony$(COLOR_RESET)"

## uninstall-service: Uninstall systemd service (requires root)
uninstall-service:
	@echo "$(COLOR_YELLOW)Uninstalling systemd service...$(COLOR_RESET)"
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "$(COLOR_YELLOW)This target requires root privileges. Run with sudo.$(COLOR_RESET)"; \
		exit 1; \
	fi
	systemctl stop packetpony 2>/dev/null || true
	systemctl disable packetpony 2>/dev/null || true
	rm -f $(SYSTEMDDIR)/packetpony.service
	systemctl daemon-reload
	@echo "$(COLOR_YELLOW)Note: User 'packetpony' and directories were not removed$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)Service uninstalled$(COLOR_RESET)"

## version: Show version information
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build time: $(BUILD_TIME)"

## docker: Build Docker image
docker:
	@echo "$(COLOR_BLUE)Building Docker image...$(COLOR_RESET)"
	docker build -t packetpony:$(VERSION) -t packetpony:latest .
	@echo "$(COLOR_GREEN)Docker image built: packetpony:$(VERSION)$(COLOR_RESET)"

## check: Run all checks (fmt, vet, lint, test)
check: fmt-check vet lint test
	@echo "$(COLOR_GREEN)All checks passed!$(COLOR_RESET)"

## ci: Run CI checks
ci: deps fmt-check vet test
	@echo "$(COLOR_GREEN)CI checks passed!$(COLOR_RESET)"
