## sshc — Makefile

APP     := sshc
MAIN_DIR := ./cmd/sshc
GOEXE = $(shell go env GOEXE)
GOPATH = $(shell go env GOPATH)
BINARY  := $(APP)$(GOEXE)

# Build metadata
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_HASH  := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev-$(GIT_HASH)")

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_HASH) \
	-X 'main.BuildDate=$(BUILD_TIME)'

.PHONY: all build backend clean help latest

DIST_DIR := dist
# 注：值会经 `echo "description: $(DESCRIPTION)"` 写入 latest.yaml，避免 `;`/`:`/引号等
# shell/YAML 元字符（否则 recipe 展开后会被截断成多条命令）。
DESCRIPTION := small SSH helper CLI for managing hosts, running remote commands.

## all: build (default)
all: build

## build: build Go binary (current platform)
build:
	@mkdir -p $(DIST_DIR)
	@echo "🐹 Building Go binary ($(VERSION) @ $(GIT_HASH))..."
	go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) $(MAIN_DIR)
	@echo "📦 Compressing binary..."
	@upx -6 --no-progress $(DIST_DIR)/$(BINARY)
	@echo "✅ Binary: $(DIST_DIR)/$(BINARY) ($$(du -sh $(DIST_DIR)/$(BINARY) | cut -f1))"

## install: install Go binary to $GOPATH/bin
install: build
	@cp $(DIST_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY)
	@echo "✅ Installed to $(GOPATH)/bin/$(BINARY)"

## run: build and run with current directory
run: build
	./$(DIST_DIR)/$(BINARY)

# ─── Cross Compilation ────────────────────────────────────────────────────────

## build-all: cross-compile for all platforms
build-all: clean-dist dump-info build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows latest-yaml
	ls -lh $(DIST_DIR)

## dump-info: dump build info
dump-info:
	@echo "Build Info:"
	@echo "  VERSION: $(VERSION)"
	@echo "  GIT_HASH: $(GIT_HASH)"
	@echo "  BUILD_TIME: $(BUILD_TIME)"

## latest-yaml: generate latest.yaml release metadata
latest-yaml:
	@mkdir -p $(DIST_DIR)
	@{ \
		echo "name: $(APP)"; \
		echo "version: $(VERSION)"; \
		echo "released_at: $(BUILD_TIME)"; \
		echo "description: $(DESCRIPTION)"; \
	} > $(DIST_DIR)/latest.yaml
	@echo "   → $(DIST_DIR)/latest.yaml"

## build-linux: compile for Linux amd64
build-linux:
	@echo "🐧 linux/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-amd64 $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-linux-amd64
	chmod +x $(DIST_DIR)/$(APP)-linux-amd64
	@echo "   → $(DIST_DIR)/$(APP)-linux-amd64"

## build-linux-arm64: compile for Linux arm64
build-linux-arm64:
	@echo "🐧 linux/arm64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-arm64 $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-linux-arm64
	chmod +x $(DIST_DIR)/$(APP)-linux-arm64
	@echo "   → $(DIST_DIR)/$(APP)-linux-arm64"

## build-darwin: compile for macOS amd64
build-darwin:
	@echo "🍎 darwin/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-amd64 $(MAIN_DIR)
	@echo "   → $(DIST_DIR)/$(APP)-darwin-amd64"

## build-darwin-arm64: compile for macOS Apple Silicon
build-darwin-arm64:
	@echo "🍎 darwin/arm64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-arm64 $(MAIN_DIR)
	# upx -6 --no-progress $(DIST_DIR)/$(APP)-darwin-arm64 # 压缩有问题在 macos 12+
	@echo "   → $(DIST_DIR)/$(APP)-darwin-arm64"

## build-windows: compile for Windows amd64
build-windows:
	@echo "🪟 windows/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-windows-amd64.exe $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-windows-amd64.exe
	@echo "   → $(DIST_DIR)/$(APP)-windows-amd64.exe"

.PHONY: release
release: build-all ## Create release archives for all platforms TODO 还未启用的
	@echo "Creating release archives..."
	@mkdir -p release
	@cd $(DIST_DIR) && \
	tar -czf ../release/$(APP)-linux-amd64.tar.gz $(APP)-linux-amd64; \
	tar -czf ../release/$(APP)-linux-arm64.tar.gz $(APP)-linux-arm64; \
	tar -czf ../release/$(APP)-darwin-amd64.tar.gz $(APP)-darwin-amd64; \
	tar -czf ../release/$(APP)-darwin-arm64.tar.gz $(APP)-darwin-arm64; \
	zip ../release/$(APP)-windows-amd64.zip $(APP)-windows-amd64.exe;
	@echo "Release archives created in release/"

## clean: remove build artifacts
clean:
	@rm -f $(BINARY)
	@rm -rf $(DIST_DIR)
	@echo "🧹 Cleaned"

## clean-dist: remove old dist files
clean-dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@echo "🧹 Cleaned $(DIST_DIR)"

## help: show this help
help:
	@echo "gofer Build System"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
