.PHONY: check test format lint build help tidy

PROJECT_BASE := github.com/0magnet/audioprism-go

# Go environment variables
GOARCH := $(shell go env GOARCH)

# Test options
TEST_OPTS := -cover -timeout=5m -mod=vendor

# Lint options
LINT_OPTS :=

check: lint test ## Run linters and tests

build: ## Install dependencies, build binary
	go build ./cmd/audioprism

lint: ## Run linters
	golangci-lint --version
	$(LINT_OPTS) golangci-lint run -c .golangci.yml ./...

test: ## Run tests if test files are present
	@echo "Checking for test files..."
	@PKG_TEST_FILES=$(shell find ./pkg -name '*_test.go' | head -n 1); \
	CMD_TEST_FILES=$(shell find ./cmd -name '*_test.go' | head -n 1); \
	if [ -n "$$PKG_TEST_FILES" ] || [ -n "$$CMD_TEST_FILES" ]; then \
		echo "Test files found. Running tests..."; \
		-go clean -testcache &>/dev/null; \
		if [ -n "$$PKG_TEST_FILES" ]; then \
			$(LINT_OPTS) go test $(TEST_OPTS) ./pkg/...; \
		fi; \
		if [ -n "$$CMD_TEST_FILES" ]; then \
			$(LINT_OPTS) go test $(TEST_OPTS) ./cmd/...; \
		fi; \
	else \
		echo "No test files found. Skipping tests."; \
	fi
	go run cmd/audioprism/audioprism.go --help
	go run cmd/fyne/fyne.go --help
	go run cmd/gomobile/gomobile.go --help

format: tidy ## Formats the code. Requires goimports and goimports-reviser
	$(LINT_OPTS) goimports -w -local $(PROJECT_BASE) ./pkg ./cmd
	find . -type f -name '*.go' -not -path "./.git/*" -not -path "./vendor/*" \
		-exec goimports-reviser -project-name $(PROJECT_BASE) {} \;

tidy: ## Clean up go module files
	go mod tidy

help: ## Display help menu
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
