# Copyright (c) 2026 dexpace and Omar Aljarrah.
# Licensed under the MIT License. See LICENSE in the repository root for details.

# Single-module Go SDK. All targets run from the repository root.

GO        ?= go
GOFLAGS   ?=
PKGS      := ./...

.DEFAULT_GOAL := check

.PHONY: check
check: tidy fmt vet lint test ## Run the full local gate (tidy, fmt, vet, lint, test).

.PHONY: build
build: ## Compile every package.
	$(GO) build $(GOFLAGS) $(PKGS)

.PHONY: test
test: ## Run tests with the race detector and coverage.
	$(GO) test $(GOFLAGS) -race -covermode=atomic -coverprofile=coverage.out $(PKGS)

.PHONY: cover
cover: test ## Open the HTML coverage report.
	$(GO) tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet $(PKGS)

.PHONY: lint
lint: ## Run golangci-lint (install: https://golangci-lint.run).
	golangci-lint run

.PHONY: fmt
fmt: ## Format with gofumpt (falls back to gofmt) and tidy imports.
	gofumpt -w . 2>/dev/null || $(GO) fmt $(PKGS)

.PHONY: tidy
tidy: ## Ensure go.mod/go.sum are current.
	$(GO) mod tidy

.PHONY: help
help: ## List available targets.
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
