# Makefile for building the project

# Use /bin/bash as the shell
SHELL := /bin/bash

# Default target
.DEFAULT_GOAL := help

# Proto files
PROTO_FILES := proto/echo.proto

# Go application name
APP_NAME := echo-app

# Docker image name
DOCKER_IMAGE := ghcr.io/philipschmid/$(APP_NAME)

# Help target
.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# Proto target
.PHONY: proto
proto: ## Generate and update the proto files
	@echo "Generating proto files..."
	protoc \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		$(PROTO_FILES)
	@echo "Proto files generated."

# Docker target
.PHONY: docker
docker: proto ## Build the Docker image
	@echo "Building Docker image..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		--build-arg VCS_REF=$(shell git rev-parse --short HEAD) \
		--tag ghcr.io/philipschmid/echo-app \
		.
	@echo "Docker image built: ghcr.io/philipschmid/echo-app"

# Test target
.PHONY: test
test: ## Run unit tests with coverage
	@echo "Running unit tests with coverage..."
	go test \
		-v \
		-cover \
		./...
	@echo "Tests completed."

# Lint target
.PHONY: lint
lint: ## Run golangci-lint with error checking
	@echo "Running golangci-lint..."
	golangci-lint run \
		--enable errcheck \
		--timeout 5m
	@echo "golangci-lint completed."

# Vet target
.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...
	@echo "go vet completed."

# Build target
.PHONY: build
build: ## Build the Go application with linting, vetting, and testing
	@echo "Building the Go application..."
	@golangci-lint run --enable errcheck > build.log 2>&1 || { echo "Linting failed, check build.log"; exit 1; }
	@go vet ./... >> build.log 2>&1 || { echo "Vet failed, check build.log"; exit 1; }
	@go test -v -cover ./... >> build.log 2>&1 || { echo "Tests failed, check build.log"; exit 1; }
	@go build \
		-ldflags="-s -w" \
		-a \
		-o echo-app \
		cmd/echo-app/main.go >> build.log 2>&1 || { echo "Build failed, check build.log"; exit 1; }
	@echo "Build completed."

# Run target
.PHONY: run
run: ## Run the Go application
	@echo "Running the Go application..."
	./$(APP_NAME)

# Run all target
.PHONY: run-all
run-all: ## Run the Go application with all listeners
	@echo "Running the Go application with all listeners..."
	./$(APP_NAME) \
		--tls \
		--tcp \
		--grpc \
		--quic

# Run all debug mode target
.PHONY: run-all-debug
run-all-debug: ## Run the Go application with all listeners in debug mode
	@echo "Running the Go application in debug mode..."
	./$(APP_NAME) \
		--tls \
		--tcp \
		--grpc \
		--quic \
		--log-level debug

# Test all endpoints target
.PHONY: test-all-endpoints
test-all-endpoints: build ## Test all endpoints after starting the app with all listeners
	@echo "Starting the application in the background..."
	@rm -f app.log
	@./echo-app \
		--tls \
		--tcp \
		--grpc \
		--quic \
		--metrics \
		--log-level info \
		--message="test-message" \
		--node="test-node" \
		--print-http-request-headers \
		> app.log 2>&1 & \
	APP_PID=$$!; \
	echo "Application PID: $$APP_PID"; \
	sleep 5; \
	echo "==========================================="; \
	echo "Testing HTTP endpoint..."; \
	echo "----- HTTP Response -----"; \
	curl -s http://localhost:8080 | jq || echo "HTTP test failed"; \
	echo "-------------------------"; \
	echo "Testing TLS endpoint..."; \
	echo "----- TLS Response -----"; \
	curl -s --insecure https://localhost:8443 | jq || echo "TLS test failed"; \
	echo "-------------------------"; \
	echo "Testing TCP endpoint..."; \
	echo "----- TCP Response -----"; \
	(echo "test" | nc localhost 9090 | jq) || echo "TCP test failed"; \
	echo "-------------------------"; \
	echo "Testing gRPC endpoint..."; \
	echo "----- gRPC Response -----"; \
	grpcurl -plaintext -emit-defaults localhost:50051 echo.EchoService.Echo | jq || echo "gRPC test failed"; \
	echo "-------------------------"; \
	echo "Testing QUIC endpoint..."; \
	echo "----- QUIC Response -----"; \
	if curl --version | grep -q "HTTP3"; then \
		curl -k -sS --http3 https://localhost:4433 | jq || echo "QUIC test failed"; \
	else \
		echo "curl does not support HTTP/3; skipping QUIC test."; \
	fi; \
	echo "-------------------------"; \
	echo "==========================================="; \
	echo "Stopping the application..."; \
	kill $$APP_PID || true; \
	while kill -0 $$APP_PID 2>/dev/null; do sleep 1; done; \
	echo "Application stopped."; \
	echo "Check app.log for application logs if needed."

# Clean up build artifacts target
.PHONY: cleanup
cleanup: ## Clean up build artifacts
	@echo "Cleaning up build artifacts..."
	rm -f $(APP_NAME) build.log app.log
	@echo "Cleaning up build artifacts completed."