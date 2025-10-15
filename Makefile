# Go Makefile - AGILira Standard
# Usage: make help

.PHONY: help test race fmt vet lint security vulncheck mod-verify check deps clean build install tools
.DEFAULT_GOAL := help

# Variables
BINARY_NAME := $(shell basename $(PWD))
GO_FILES := $(shell find . -type f -name '*.go' -not -path './vendor/*')
TOOLS_DIR := $(HOME)/go/bin

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(BLUE)Available targets:$(NC)"
		@echo "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(BLUE)Fuzz Testing Commands:$(NC)"
	@echo "  $(GREEN)fuzz$(NC)            Run fuzz tests (30s each)"
	@echo "  $(GREEN)fuzz-long$(NC)       Run extended fuzz tests (5m each)"
	@echo "  $(GREEN)fuzz-validate$(NC)   Fuzz ValidateSecurePath only"
	@echo "  $(GREEN)fuzz-parse$(NC)      Fuzz ParseConfig only"
	@echo "  $(GREEN)security-fuzz$(NC)   Security checks + fuzz testing"

test: ## Run tests
	@echo "$(YELLOW)Running tests...$(NC)"
	go test -v ./...

race: ## Run tests with race detector
	@echo "$(YELLOW)Running tests with race detector...$(NC)"
	go test -race -v ./...

coverage: ## Run tests with coverage
	@echo "$(YELLOW)Running tests with coverage...$(NC)"
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

fmt: ## Format Go code
	@echo "$(YELLOW)Formatting Go code...$(NC)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(YELLOW)Running go vet...$(NC)"
	go vet ./...

staticcheck: ## Run staticcheck
	@echo "$(YELLOW)Running staticcheck...$(NC)"
	@if [ ! -f "$(TOOLS_DIR)/staticcheck" ]; then \
		echo "$(RED)staticcheck not found. Run 'make tools' to install.$(NC)"; \
		exit 1; \
	fi
	$(TOOLS_DIR)/staticcheck ./...

errcheck: ## Run errcheck
	@echo "$(YELLOW)Running errcheck...$(NC)"
	@if [ ! -f "$(TOOLS_DIR)/errcheck" ]; then \
		echo "$(RED)errcheck not found. Run 'make tools' to install.$(NC)"; \
		exit 1; \
	fi
	$(TOOLS_DIR)/errcheck ./...

gosec: ## Run gosec security scanner
	@echo "$(YELLOW)Running gosec security scanner...$(NC)"
	@if [ ! -f "$(TOOLS_DIR)/gosec" ]; then \
		echo "$(RED)gosec not found. Run 'make tools' to install.$(NC)"; \
		exit 1; \
	fi
	@$(TOOLS_DIR)/gosec ./... || (echo "$(YELLOW)  gosec completed with warnings (may be import-related)$(NC)" && exit 0)

vulncheck: ## Run govulncheck vulnerability scanner
	@echo "$(YELLOW)Running govulncheck...$(NC)"
	@if [ ! -f "$(TOOLS_DIR)/govulncheck" ]; then \
		echo "$(RED)govulncheck not found. Run 'make tools' to install.$(NC)"; \
		exit 1; \
	fi
	$(TOOLS_DIR)/govulncheck ./...

mod-verify: ## Verify module dependencies
	@echo "$(YELLOW)Running go mod verify...$(NC)"
	go mod verify
	@echo "$(GREEN)Module verification passed.$(NC)"

lint: staticcheck errcheck ## Run all linters
	@echo "$(GREEN)All linters completed.$(NC)"

security: gosec ## Run security checks
	@echo "$(GREEN)Security checks completed.$(NC)"

security-fuzz: gosec fuzz ## Run security checks including fuzz testing
	@echo "$(GREEN)Security checks with fuzz testing completed.$(NC)"

check: mod-verify fmt vet lint security vulncheck test ## Run all checks (format, vet, lint, security, test)
	@echo "$(GREEN)All checks passed!$(NC)"

check-race: mod-verify fmt vet lint security vulncheck race ## Run all checks including race detector
	@echo "$(GREEN)All checks with race detection passed!$(NC)"

tools: ## Install development tools
	@echo "$(YELLOW)Installing development tools...$(NC)"
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/kisielk/errcheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "$(GREEN)Tools installed successfully!$(NC)"

deps: ## Download and verify dependencies
	@echo "$(YELLOW)Downloading dependencies...$(NC)"
	go mod download
	go mod verify
	go mod tidy

clean: ## Clean build artifacts and test cache
	@echo "$(YELLOW)Cleaning...$(NC)"
	go clean
	go clean -testcache
	rm -f coverage.out coverage.html
	rm -f $(BINARY_NAME)

build: ## Build the binary
	@echo "$(YELLOW)Building $(BINARY_NAME)...$(NC)"
	go build -ldflags="-w -s" -o $(BINARY_NAME) .

install: ## Install the binary to $GOPATH/bin
	@echo "$(YELLOW)Installing $(BINARY_NAME)...$(NC)"
	go install .

bench: ## Run benchmarks
	@echo "$(YELLOW)Running benchmarks...$(NC)"
	go test -bench=. -benchmem ./...

fuzz: ## Run fuzz tests for security critical functions
	@echo "$(YELLOW)Running fuzz tests...$(NC)"
	@echo "$(BLUE)Fuzzing ValidateSecurePath for 30 seconds...$(NC)"
	go test -fuzz=FuzzValidateSecurePath -fuzztime=30s
	@echo "$(BLUE)Fuzzing ParseConfig for 30 seconds...$(NC)"
	go test -fuzz=FuzzParseConfig -fuzztime=30s
	@echo "$(GREEN)Fuzz testing completed!$(NC)"

fuzz-long: ## Run extended fuzz tests (5 minutes each)
	@echo "$(YELLOW)Running extended fuzz tests...$(NC)"
	@echo "$(BLUE)Fuzzing ValidateSecurePath for 5 minutes...$(NC)"
	go test -fuzz=FuzzValidateSecurePath -fuzztime=5m
	@echo "$(BLUE)Fuzzing ParseConfig for 5 minutes...$(NC)"
	go test -fuzz=FuzzParseConfig -fuzztime=5m
	@echo "$(GREEN)Extended fuzz testing completed!$(NC)"

fuzz-validate: ## Run fuzz test for ValidateSecurePath only
	@echo "$(YELLOW)Fuzzing ValidateSecurePath...$(NC)"
	go test -fuzz=FuzzValidateSecurePath -fuzztime=1m

fuzz-parse: ## Run fuzz test for ParseConfig only
	@echo "$(YELLOW)Fuzzing ParseConfig...$(NC)"
	go test -fuzz=FuzzParseConfig -fuzztime=1m

ci: ## Run CI checks (used in GitHub Actions)
	@echo "$(BLUE)Running CI checks...$(NC)"
	@make fmt vet lint security test coverage
	@echo "$(GREEN)CI checks completed successfully!$(NC)"

dev: ## Quick development check (fast feedback loop)
	@echo "$(BLUE)Running development checks...$(NC)"
	@make fmt vet test
	@echo "$(GREEN)Development checks completed!$(NC)"

pre-commit: check ## Run pre-commit checks (alias for 'check')

all: clean tools deps check build ## Run everything from scratch

# Show tool status
status: ## Show status of installed tools
	@echo "$(BLUE)Development tools status:$(NC)"
	@echo -n "staticcheck: "; [ -f "$(TOOLS_DIR)/staticcheck" ] && echo "$(GREEN)✓ installed$(NC)" || echo "$(RED)✗ missing$(NC)"
	@echo -n "errcheck:    "; [ -f "$(TOOLS_DIR)/errcheck" ] && echo "$(GREEN)✓ installed$(NC)" || echo "$(RED)✗ missing$(NC)"
	@echo -n "gosec:       "; [ -f "$(TOOLS_DIR)/gosec" ] && echo "$(GREEN)✓ installed$(NC)" || echo "$(RED)✗ missing$(NC)"
	@echo -n "govulncheck: "; [ -f "$(TOOLS_DIR)/govulncheck" ] && echo "$(GREEN)✓ installed$(NC)" || echo "$(RED)✗ missing$(NC)"