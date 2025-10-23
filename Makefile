# Binário e pacote principal
BINARY ?= aws-finops
MAIN_PKG := ./cmd/aws-finops

# Pasta de saída
BIN_DIR ?= bin

# Detecta valores do Git (podem ser sobrescritos por env vars)
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "0.0.0-dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILDTIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Marca sufixo -dirty se houver modificações não commitadas
GIT_DIRTY := $(shell test -n "$$(git status --porcelain 2>/dev/null)" && echo "-dirty" )
VERSION := $(VERSION)$(GIT_DIRTY)

# Flags de linkagem (ldflags)
LDFLAGS := -s -w \
	-X github.com/diillson/aws-finops-dashboard-go/pkg/version.Version=$(VERSION) \
	-X github.com/diillson/aws-finops-dashboard-go/pkg/version.Commit=$(COMMIT) \
	-X github.com/diillson/aws-finops-dashboard-go/pkg/version.BuildTime=$(BUILDTIME)

# Go env
GO ?= go
GOFLAGS ?=
CGO_ENABLED ?= 0

# Lista de plataformas para release (edite conforme necessidade)
RELEASE_PLATFORMS ?= \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: all build build-dev clean release print-version

all: build

print-version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILDTIME)"

build:
	@mkdir -p $(BIN_DIR)
	@echo "Building $(BINARY) with ldflags (Version=$(VERSION), Commit=$(COMMIT))..."
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "Binary: $(BIN_DIR)/$(BINARY)"
	@$(BIN_DIR)/$(BINARY) --version || true

# Build sem ldflags: o pacote version usará o fallback via debug.ReadBuildInfo
build-dev:
	@mkdir -p $(BIN_DIR)
	@echo "Building $(BINARY) (dev) without ldflags..."
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) $(MAIN_PKG)
	@echo "Binary: $(BIN_DIR)/$(BINARY)"
	@$(BIN_DIR)/$(BINARY) --version || true

# Build de múltiplas plataformas (binários nomeados com OS-ARCH)
release:
	@mkdir -p $(BIN_DIR)
	@set -e; \
	for plat in $(RELEASE_PLATFORMS); do \
			OS=$${plat%/*}; ARCH=$${plat*/}; \
			OUT="$(BIN_DIR)/$(BINARY)-$${OS}-$${ARCH}"; \
			if [ "$$OS" = "windows" ]; then OUT="$$OUT.exe"; fi; \
			echo "Building $$OUT (Version=$(VERSION), Commit=$(COMMIT))"; \
			GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=$(CGO_ENABLED) \
			$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o "$$OUT" $(MAIN_PKG); \
	done
	@echo "Artifacts in: $(BIN_DIR)"

clean:
	@rm -rf $(BIN_DIR)
	@echo "Cleaned $(BIN_DIR)"