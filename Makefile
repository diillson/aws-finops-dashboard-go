# Makefile

# Diretórios
BIN_DIR := ./bin
DIST_DIR := ./dist

# Nome do executável
BINARY_NAME := aws-finops

# Informações da versão
VERSION := $(shell grep -m1 '^const Version =' pkg/version/version.go | cut -d '"' -f2)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go flags
GO_FLAGS := -ldflags "-X github.com/diillson/aws-finops-dashboard-go/pkg/version.Commit=$(COMMIT) -X github.com/diillson/aws-finops-dashboard-go/pkg/version.BuildTime=$(BUILD_TIME)"

.PHONY: all build release clean test lint fmt help install uninstall

all: clean lint test build

# Compila o projeto
build:
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	@mkdir -p $(BIN_DIR)
	@go build $(GO_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) cmd/aws-finops/main.go
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Cria o release
release:
	@echo "Releasing version $(VERSION)"
	git tag -a v$(VERSION) -m "Version $(VERSION)"
	git push origin v$(VERSION)

# Limpa o diretório de build
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR) $(DIST_DIR)
	@go clean
	@echo "Clean complete"

# Roda os testes
test:
	@echo "Running tests..."
	@go test -v ./...

# Roda o linter
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run

# Formata o código
fmt:
	@echo "Formatting code..."
	@gofmt -w -s .
	@echo "Format complete"

# Instala o binário
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BIN_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Install complete: $(GOPATH)/bin/$(BINARY_NAME)"

# Desinstala o binário
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Uninstall complete"

# Ajuda
help:
	@echo "Available targets:"
	@echo "  all        : Clean, lint, test and build"
	@echo "  build      : Build the binary"
	@echo "  release    : Create a new release"
	@echo "  clean      : Clean build artifacts"
	@echo "  test       : Run tests"
	@echo "  lint       : Run golangci-lint"
	@echo "  fmt        : Format code"
	@echo "  install    : Install binary to GOPATH/bin"
	@echo "  uninstall  : Remove binary from GOPATH/bin"
	@echo "  help       : Show this help"
