# Makefile for building the project
.PHONY: proto docker test lint vet build run

# Use /bin/bash as the shell
SHELL := /bin/bash

# Default target
.DEFAULT_GOAL := help

# Proto files
PROTO_FILES := echo.proto

# Go application name
APP_NAME := echo-app

# Docker image name
DOCKER_IMAGE := ghcr.io/philipschmid/$(APP_NAME)

# Help target
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

# Proto target
proto: ## Generate and update the proto files
	@echo "Generating proto files..."
	protoc --go_out=. --go-grpc_out=. $(PROTO_FILES)
	@echo "Proto files generated."

# Docker target
docker: proto ## Build the Docker image
	@echo "Building Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 --tag $(DOCKER_IMAGE) .
	@echo "Docker image built: $(DOCKER_IMAGE)"

# Test target
test: ## Run tests
	@echo "Running tests..."
	go test -v ./...
	@echo "Tests completed."

# Lint target
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	golangci-lint run
	@echo "golangci-lint completed."

# Vet target
vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...
	@echo "go vet completed."

# Build target
build: lint vet test ## Build the Go application
	@echo "Building the Go application..."
	go build -o $(APP_NAME) .
	@echo "Build completed."

# Run target
run: ## Run the Go application
	@echo "Running the Go application..."
	./$(APP_NAME)

# Run all target
run-all: ## Run the Go application with all listeners
	@echo "Running the Go application..."
	TLS="true" TCP="true" GRPC="true" QUIC="true" $(MAKE) run

# Run all debug mode target
run-all-debug: ## Run the Go application with all listeners in debug mode
	@echo "Running the Go application in debug mode..."
	LOG_LEVEL="debug" $(MAKE) run-all

# Clean up build artifcats target
cleanup: ## Clean up build artifacts
	@echo "Cleaning up build artifacts"
	rm $(APP_NAME)
	@echo "Cleaning up build artifacts completed."