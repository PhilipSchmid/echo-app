# Makefile for building the project
.PHONY: proto docker

# Use /bin/bash as the shell
SHELL := /bin/bash

# Default target
.DEFAULT_GOAL := help

# Proto files
PROTO_FILES := echo.proto

# Docker image name
DOCKER_IMAGE := ghcr.io/philipschmid/echo-app

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
