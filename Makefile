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

.PHONY: all build clean test coverage lint fmt vet run install uninstall help
.PHONY: release cross-compile docker deps update-deps
.PHONY: install-service uninstall-service

# Default target
all: clean fmt vet test build

## help: Display this help message
help:
	@echo "PacketPony Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/^## /  /' | sed 's/:/ -/'
	@echo ""

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BINARY_NAME)"

## build-debug: Build with debug symbols
build-debug:
	@echo "Building $(BINARY_NAME) with debug symbols..."
	$(GOBUILD) -gcflags="all=-N -l" -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Debug build complete: $(BINARY_NAME)"

## release: Build optimized release binary
release:
	@echo "Building release version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Release build complete: $(BUILD_DIR)/$(BINARY_NAME)"

## cross-compile: Build for multiple platforms
cross-compile:
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -trimpath -o $(BUILD_DIR)/$(BINARY_NAME)-freebsd-amd64 $(MAIN_PATH)
	@echo "Cross-compilation complete"
	@ls -lh $(BUILD_DIR)/

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

## test-short: Run tests without race detection
test-short:
	@echo "Running tests (short)..."
	$(GOTEST) -v -short ./...

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	$(GOTEST) -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run linters (requires golangci-lint)
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/" && exit 1)
	golangci-lint run --timeout=5m

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@echo "Format complete"

## fmt-check: Check if code is formatted
fmt-check:
	@echo "Checking code format..."
	@test -z "$$($(GOFMT) -s -l . | tee /dev/stderr)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)
	@echo "Code is properly formatted"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...
	@echo "Vet complete"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded"

## update-deps: Update dependencies
update-deps:
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "Dependencies updated"

## tidy: Tidy go.mod
tidy:
	@echo "Tidying go.mod..."
	$(GOMOD) tidy
	@echo "Tidy complete"

## run: Build and run with example config
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME) -config configs/example.yaml

## run-test: Build and run with test config
run-test: build
	@echo "Running $(BINARY_NAME) with test config..."
	./$(BINARY_NAME) -config configs/test.yaml

## install: Install binary and config
install: build
	@echo "Installing $(BINARY_NAME)..."
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY_NAME) $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	@if [ ! -f $(DESTDIR)$(CONFIGDIR)/config.yaml ]; then \
		install -d $(DESTDIR)$(CONFIGDIR); \
		install -m 644 configs/example.yaml $(DESTDIR)$(CONFIGDIR)/config.yaml; \
		echo "Config installed to $(CONFIGDIR)/config.yaml"; \
	else \
		echo "Config already exists at $(CONFIGDIR)/config.yaml (not overwriting)"; \
	fi
	@echo "Installation complete"

## uninstall: Uninstall binary
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	@echo "Note: Config files in $(CONFIGDIR) were not removed"
	@echo "Uninstall complete"

## install-service: Install systemd service (requires root)
install-service: install
	@echo "Installing systemd service..."
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "This target requires root privileges. Run with sudo."; \
		exit 1; \
	fi
	@if ! id packetpony >/dev/null 2>&1; then \
		useradd -r -s /bin/false -d /var/lib/packetpony packetpony; \
		echo "Created user 'packetpony'"; \
	fi
	install -d /var/lib/packetpony
	install -d /var/log/packetpony
	chown packetpony:packetpony /var/lib/packetpony
	chown packetpony:packetpony /var/log/packetpony
	chown root:packetpony $(CONFIGDIR)/config.yaml
	chmod 640 $(CONFIGDIR)/config.yaml
	install -m 644 deployment/systemd/packetpony.service $(SYSTEMDDIR)/packetpony.service
	systemctl daemon-reload
	@echo "Service installed. Enable with: systemctl enable packetpony"
	@echo "Start with: systemctl start packetpony"

## uninstall-service: Uninstall systemd service (requires root)
uninstall-service:
	@echo "Uninstalling systemd service..."
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "This target requires root privileges. Run with sudo."; \
		exit 1; \
	fi
	systemctl stop packetpony 2>/dev/null || true
	systemctl disable packetpony 2>/dev/null || true
	rm -f $(SYSTEMDDIR)/packetpony.service
	systemctl daemon-reload
	@echo "Note: User 'packetpony' and directories were not removed"
	@echo "Service uninstalled"

## version: Show version information
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build time: $(BUILD_TIME)"

## docker: Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t packetpony:$(VERSION) -t packetpony:latest .
	@echo "Docker image built: packetpony:$(VERSION)"

## check: Run all checks (fmt, vet, lint, test)
check: fmt-check vet lint test
	@echo "All checks passed!"

## ci: Run CI checks
ci: deps fmt-check vet test
	@echo "CI checks passed!"
