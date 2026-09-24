SHELL := /bin/bash

APP := insights-worker
CMD := ./cmd/insights-worker
BIN_DIR := bin
COVERAGE_DIR := coverage
COVERAGE_FILE := $(COVERAGE_DIR)/coverage.out
PKGS := ./internal/... ./cmd/insights-worker

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: all
all: check build ## Run checks and build the binary

.PHONY: setup
setup: download tidy ## Download dependencies and tidy go.mod/go.sum

.PHONY: download
download: ## Download Go module dependencies
	go mod download

.PHONY: tidy
tidy: ## Tidy Go module dependencies
	go mod tidy

.PHONY: fmt
fmt: ## Format Go packages used by the worker
	go fmt $(PKGS)

.PHONY: vet
vet: ## Run go vet
	go vet $(PKGS)

.PHONY: test
test: ## Run tests
	go test $(PKGS)

.PHONY: test-v
test-v: ## Run tests with verbose output
	go test -v $(PKGS)

.PHONY: test-race
test-race: ## Run tests with the race detector
	go test -race $(PKGS)

.PHONY: test-cover
test-cover: ## Run tests and write a coverage profile
	mkdir -p $(COVERAGE_DIR)
	go test $(PKGS) -coverprofile=$(COVERAGE_FILE)

.PHONY: coverage-html
coverage-html: test-cover ## Open the coverage profile in the browser
	go tool cover -html=$(COVERAGE_FILE)

.PHONY: check
check: fmt vet test ## Run format, vet, and tests

.PHONY: build
build: ## Build the worker binary
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP) $(CMD)

.PHONY: run
run: ## Run the worker locally
	go run $(CMD)

.PHONY: clean
clean: ## Remove generated files
	go clean
	rm -rf $(BIN_DIR) $(COVERAGE_DIR)

.PHONY: docker-build
docker-build: ## Build Docker images
	docker compose build

.PHONY: docker-up
docker-up: ## Build and run Docker services in the foreground
	docker compose up --build

.PHONY: docker-up-d
docker-up-d: ## Build and run Docker services in the background
	docker compose up --build -d

.PHONY: docker-down
docker-down: ## Stop Docker services
	docker compose down

.PHONY: docker-down-volumes
docker-down-volumes: ## Stop Docker services and remove volumes/orphans
	docker compose down --volumes --remove-orphans

.PHONY: docker-logs
docker-logs: ## Follow Docker service logs
	docker compose logs -f

.PHONY: docker-ps
docker-ps: ## List Docker services
	docker compose ps

.PHONY: docker-restart
docker-restart: docker-down docker-up ## Restart Docker services in the foreground
