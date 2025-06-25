# Makefile for Namira Core

.PHONY: build up down logs clean dev prod health test lint help install run-local docker-build docker-push
.DEFAULT_GOAL := help

# Variables
VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_SHA ?= $(shell git rev-parse --short HEAD)
GO_FILES := $(shell find . -name "*.go" -type f -not -path "./vendor/*")
APP_NAME := namira-core
BINARY := ./bin/$(APP_NAME)
DOCKER_IMAGE := namiranet/$(APP_NAME)

# Common docker-compose command
DOCKER_COMPOSE := docker-compose
DOCKER_BUILD_ARGS := VERSION=$(VERSION) BUILD_DATE=$(BUILD_DATE) COMMIT_SHA=$(COMMIT_SHA)

# Development
dev: ## Start development environment with Docker Compose
	@echo "Starting development environment..."
	$(DOCKER_COMPOSE) up --build

# Production
prod: ## Start production environment with Docker Compose
	@echo "Starting production environment..."
	$(DOCKER_BUILD_ARGS) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# Build only Docker
build: ## Build Docker containers without starting them
	@echo "Building Namira Core..."
	$(DOCKER_BUILD_ARGS) $(DOCKER_COMPOSE) build

# Local Go build
build-local: ## Build the Go binary locally
	@echo "Building local binary..."
	@mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.commitSha=$(COMMIT_SHA)" -o $(BINARY) ./cmd/namira

# Run local binary
run-local: build-local ## Run the application locally
	@echo "Running local binary..."
	$(BINARY)

# Test targets
test: ## Run all tests
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint code
lint: ## Run linters
	@echo "Linting code..."
	golangci-lint run

# Start services
up: ## Start Docker Compose services
	$(DOCKER_COMPOSE) up -d

# Stop services
down: ## Stop Docker Compose services
	$(DOCKER_COMPOSE) down

# View logs
logs: ## View Docker Compose logs
	$(DOCKER_COMPOSE) logs -f $(APP_NAME)

# Clean up
clean: ## Clean up Docker resources and build artifacts
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker system prune -f
	rm -rf bin/ coverage.out coverage.html

# Health check
health: ## Check service health
	@echo "Checking service health..."
	@curl -f http://localhost:8080/health || echo "Service not healthy"

# Docker image operations
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(VERSION) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  --build-arg COMMIT_SHA=$(COMMIT_SHA) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest

docker-push: docker-build ## Build and push Docker image
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

# Install dependencies
install: ## Install project dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Show help
help: ## Show this help message
	@echo "Namira Core Makefile"
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'