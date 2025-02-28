# Makefile for ScanX Indexer

# Version to hardcode into the binary
VERSION = v0.0.1

# Binary name
BINARY = scanx

# Source directory
SRC_DIR = ./cmd/scanx

# Output directory
BUILD_DIR = ./build

# Build flags to embed version
LDFLAGS = -ldflags "-X main.Version=$(VERSION)"

# Ensure build directory exists
$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

# Default target: build for local OS into ./build
build: $(BUILD_DIR)
	@echo "Building $(BINARY) for local OS with version $(VERSION) into $(BUILD_DIR)..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(SRC_DIR)

# Build for macOS into ./build
mac: $(BUILD_DIR)
	@echo "Building $(BINARY) for macOS with version $(VERSION) into $(BUILD_DIR)..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-mac $(SRC_DIR)

# Build for Linux into ./build
linux: $(BUILD_DIR)
	@echo "Building $(BINARY) for Linux with version $(VERSION) into $(BUILD_DIR)..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux $(SRC_DIR)

# Build for Windows into ./build
windows: $(BUILD_DIR)
	@echo "Building $(BINARY) for Windows with version $(VERSION) into $(BUILD_DIR)..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY).exe $(SRC_DIR)

# Clean up build directory
clean:
	@echo "Cleaning up build directory..."
	rm -rf $(BUILD_DIR)

# Phony targets
.PHONY: build mac linux windows clean