.PHONY: all clean build-all build-linux build-macos build-windows release

# Project information
PROJECT_NAME := notify
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Build directories
BUILD_DIR := ./build
RELEASE_DIR := ./release/$(PROJECT_NAME)

# Default target
all: clean build-all

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR) $(RELEASE_DIR)
	mkdir -p $(BUILD_DIR) $(RELEASE_DIR)

# Build for all platforms
build-all: build-linux build-macos build-windows

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-linux-amd64 main.go
	cp $(BUILD_DIR)/$(PROJECT_NAME)-linux-amd64 $(RELEASE_DIR)/$(PROJECT_NAME)-linux-amd64

# Build for macOS
build-macos:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-darwin-arm64 main.go
	cp $(BUILD_DIR)/$(PROJECT_NAME)-darwin-amd64 $(RELEASE_DIR)/$(PROJECT_NAME)-darwin-amd64
	cp $(BUILD_DIR)/$(PROJECT_NAME)-darwin-arm64 $(RELEASE_DIR)/$(PROJECT_NAME)-darwin-arm64

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-windows-amd64.exe main.go
	GOOS=windows GOARCH=386 go build $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-windows-386.exe main.go
	cp $(BUILD_DIR)/$(PROJECT_NAME)-windows-amd64.exe $(RELEASE_DIR)/$(PROJECT_NAME)-windows-amd64.exe
	cp $(BUILD_DIR)/$(PROJECT_NAME)-windows-386.exe $(RELEASE_DIR)/$(PROJECT_NAME)-windows-386.exe

# Create release package with config and README
release: build-all
	@echo "Creating release package..."
	cp config.yaml $(RELEASE_DIR)/config.yaml
	cp README.md $(RELEASE_DIR)/README.md
	cp README_en.md $(RELEASE_DIR)/README_en.md 