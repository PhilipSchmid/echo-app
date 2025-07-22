# Makefile for echo-app
# A versatile Go application that echoes request information across multiple protocols

# Use /bin/bash as the shell
SHELL := /bin/bash

# Default target
.DEFAULT_GOAL := help

# Variables
APP_NAME := echo-app
DOCKER_IMAGE := ghcr.io/philipschmid/$(APP_NAME)
PROTO_FILES := proto/echo.proto
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")
GO_PACKAGES := $(shell go list ./... | grep -v /vendor/)
COVERAGE_FILE := coverage.out
BUILD_DIR := build
LDFLAGS := -ldflags="-s -w"

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

# Version information
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Help target - improved with categories
.PHONY: help
help: ## Show this help message
	@printf "$(BLUE)echo-app Makefile$(NC)\n"
	@printf "$(BLUE)=================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)Usage:$(NC) make [target]\n"
	@printf "\n"
	@printf "$(YELLOW)Development:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## DEV:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## DEV:"}; {printf "  %-20s %s\n", $$1, $$2}'
	@printf "\n"
	@printf "$(YELLOW)Building:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## BUILD:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## BUILD:"}; {printf "  %-20s %s\n", $$1, $$2}'
	@printf "\n"
	@printf "$(YELLOW)Testing:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## TEST:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## TEST:"}; {printf "  %-20s %s\n", $$1, $$2}'
	@printf "\n"
	@printf "$(YELLOW)Running:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## RUN:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## RUN:"}; {printf "  %-20s %s\n", $$1, $$2}'
	@printf "\n"
	@printf "$(YELLOW)Docker:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## DOCKER:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## DOCKER:"}; {printf "  %-20s %s\n", $$1, $$2}'
	@printf "\n"
	@printf "$(YELLOW)Maintenance:$(NC)\n"
	@grep -E '^[a-zA-Z_-]+:.*?## MAINT:' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## MAINT:"}; {printf "  %-20s %s\n", $$1, $$2}'

# Development targets
.PHONY: dev
dev: ## DEV: Run the application with file watching (requires entr)
	@command -v entr >/dev/null 2>&1 || { printf "$(RED)Error: entr is not installed. Install it with: brew install entr$(NC)\n"; exit 1; }
	@printf "$(BLUE)Starting development mode with file watching...$(NC)\n"
	@find . -name "*.go" | entr -r make run

.PHONY: proto
proto: ## DEV: Generate protobuf files
	@printf "$(BLUE)Generating proto files...$(NC)\n"
	@command -v protoc >/dev/null 2>&1 || { printf "$(RED)Error: protoc is not installed$(NC)\n"; exit 1; }
	@protoc \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		$(PROTO_FILES)
	@printf "$(GREEN)✓ Proto files generated$(NC)\n"

.PHONY: deps
deps: ## DEV: Download and verify dependencies
	@printf "$(BLUE)Downloading dependencies...$(NC)\n"
	@go mod download
	@go mod verify
	@printf "$(GREEN)✓ Dependencies downloaded and verified$(NC)\n"

.PHONY: tidy
tidy: ## DEV: Tidy and vendor dependencies
	@printf "$(BLUE)Tidying dependencies...$(NC)\n"
	@go mod tidy
	@printf "$(GREEN)✓ Dependencies tidied$(NC)\n"

# Building targets
.PHONY: build
build: ## BUILD: Build the application with all checks
	@printf "$(BLUE)Building $(APP_NAME) v$(VERSION)...$(NC)\n"
	@mkdir -p $(BUILD_DIR)
	@printf "$(YELLOW)→ Running linter...$(NC)\n"
	@golangci-lint run --enable errcheck > $(BUILD_DIR)/lint.log 2>&1 || { printf "$(RED)✗ Linting failed, check $(BUILD_DIR)/lint.log$(NC)\n"; exit 1; }
	@printf "$(GREEN)✓ Linting passed$(NC)\n"
	@printf "$(YELLOW)→ Running go vet...$(NC)\n"
	@go vet ./... > $(BUILD_DIR)/vet.log 2>&1 || { printf "$(RED)✗ Vet failed, check $(BUILD_DIR)/vet.log$(NC)\n"; exit 1; }
	@printf "$(GREEN)✓ Vet passed$(NC)\n"
	@printf "$(YELLOW)→ Running tests...$(NC)\n"
	@go test -v -cover ./... > $(BUILD_DIR)/test.log 2>&1 || { printf "$(RED)✗ Tests failed, check $(BUILD_DIR)/test.log$(NC)\n"; exit 1; }
	@printf "$(GREEN)✓ Tests passed$(NC)\n"
	@printf "$(YELLOW)→ Building binary...$(NC)\n"
	@go build \
		$(LDFLAGS) \
		-a \
		-o $(APP_NAME) \
		cmd/echo-app/main.go
	@printf "$(GREEN)✓ Build completed: $(APP_NAME)$(NC)\n"

.PHONY: build-quick
build-quick: ## BUILD: Quick build without checks
	@printf "$(BLUE)Quick building $(APP_NAME)...$(NC)\n"
	@go build $(LDFLAGS) -o $(APP_NAME) cmd/echo-app/main.go
	@printf "$(GREEN)✓ Quick build completed$(NC)\n"

.PHONY: build-all
build-all: ## BUILD: Build for all platforms
	@printf "$(BLUE)Building for all platforms...$(NC)\n"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 cmd/echo-app/main.go
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 cmd/echo-app/main.go
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 cmd/echo-app/main.go
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 cmd/echo-app/main.go
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe cmd/echo-app/main.go
	@printf "$(GREEN)✓ Built for all platforms$(NC)\n"
	@ls -la $(BUILD_DIR)/

# Testing targets
.PHONY: test
test: ## TEST: Run unit tests with coverage
	@printf "$(BLUE)Running unit tests...$(NC)\n"
	@go test -v -race -cover ./...
	@printf "$(GREEN)✓ Tests completed$(NC)\n"

.PHONY: test-coverage
test-coverage: ## TEST: Run tests with detailed coverage report
	@printf "$(BLUE)Running tests with coverage analysis...$(NC)\n"
	@go test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -html=$(COVERAGE_FILE) -o $(BUILD_DIR)/coverage.html
	@printf "$(GREEN)✓ Coverage report generated: $(BUILD_DIR)/coverage.html$(NC)\n"
	@printf "$(YELLOW)Coverage summary:$(NC)\n"
	@go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print "Total coverage: " $$3}'

.PHONY: test-short
test-short: ## TEST: Run only short tests
	@printf "$(BLUE)Running short tests...$(NC)\n"
	@go test -v -short ./...
	@printf "$(GREEN)✓ Short tests completed$(NC)\n"

.PHONY: test-integration
test-integration: build-quick ## TEST: Run integration tests with all endpoints
	@printf "$(BLUE)Starting integration tests...$(NC)\n"
	@mkdir -p $(BUILD_DIR)
	@rm -f $(BUILD_DIR)/app.log $(BUILD_DIR)/test-results.log
	@./$(APP_NAME) \
		--tls \
		--tcp \
		--grpc \
		--quic \
		--metrics \
		--log-level info \
		--message="integration-test" \
		--node="test-node" \
		--print-http-request-headers \
		> $(BUILD_DIR)/app.log 2>&1 & \
	APP_PID=$$!; \
	printf "$(YELLOW)→ Application started with PID: $$APP_PID$(NC)\n"; \
	printf "$(YELLOW)→ Waiting for services to start...$(NC)\n"; \
	sleep 3; \
	\
	echo "" > $(BUILD_DIR)/test-results.log; \
	FAILED=0; \
	\
	printf "$(BLUE)Testing HTTP endpoint (port 8080)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if curl -s http://localhost:8080 | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
		printf "$(GREEN)✓ HTTP test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ HTTP test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing HTTP with custom path...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if curl -s http://localhost:8080/test/path | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
		printf "$(GREEN)✓ HTTP path test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ HTTP path test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing TLS endpoint (port 8443)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if curl -s --insecure https://localhost:8443 | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
		printf "$(GREEN)✓ TLS test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ TLS test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing TCP endpoint (port 9090)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if echo "test" | nc -w 2 localhost 9090 | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
		printf "$(GREEN)✓ TCP test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ TCP test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing gRPC endpoint (port 50051)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if command -v grpcurl >/dev/null 2>&1; then \
		if grpcurl -plaintext -emit-defaults localhost:50051 echo.EchoService.Echo | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
			printf "$(GREEN)✓ gRPC test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		else \
			printf "$(RED)✗ gRPC test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
			FAILED=$$((FAILED + 1)); \
		fi; \
	else \
		printf "$(YELLOW)⚠ grpcurl not installed, skipping gRPC test$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	fi; \
	\
	printf "$(BLUE)Testing QUIC endpoint (port 4433)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if curl --version | grep -q "HTTP3"; then \
		if curl -k -sS --http3 https://localhost:4433 | jq . >> $(BUILD_DIR)/test-results.log 2>&1; then \
			printf "$(GREEN)✓ QUIC test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		else \
			printf "$(RED)✗ QUIC test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
			FAILED=$$((FAILED + 1)); \
		fi; \
	else \
		printf "$(YELLOW)⚠ curl does not support HTTP/3, skipping QUIC test$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	fi; \
	\
	printf "$(BLUE)Testing metrics endpoint (port 3000)...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if curl -s http://localhost:3000/metrics | grep -q "echo_app_requests_total"; then \
		printf "$(GREEN)✓ Metrics test passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ Metrics test failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing health endpoint...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if [ "$$(curl -s http://localhost:3000/health)" = "OK" ]; then \
		printf "$(GREEN)✓ Health check passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ Health check failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing readiness endpoint...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	if [ "$$(curl -s http://localhost:3000/ready)" = "Ready" ]; then \
		printf "$(GREEN)✓ Readiness check passed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	else \
		printf "$(RED)✗ Readiness check failed$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
		FAILED=$$((FAILED + 1)); \
	fi; \
	\
	printf "$(BLUE)Testing graceful shutdown...$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	kill -TERM $$APP_PID; \
	SHUTDOWN_START=$$(date +%s); \
	while kill -0 $$APP_PID 2>/dev/null; do \
		sleep 0.1; \
	done; \
	SHUTDOWN_END=$$(date +%s); \
	SHUTDOWN_TIME=$$((SHUTDOWN_END - SHUTDOWN_START)); \
	printf "$(GREEN)✓ Application shut down gracefully in $$SHUTDOWN_TIME seconds$(NC)\n" | tee -a $(BUILD_DIR)/test-results.log; \
	\
	echo ""; \
	printf "$(BLUE)========================================$(NC)\n"; \
	if [ $$FAILED -eq 0 ]; then \
		printf "$(GREEN)✓ All integration tests passed!$(NC)\n"; \
		exit 0; \
	else \
		printf "$(RED)✗ $$FAILED integration tests failed$(NC)\n"; \
		printf "$(YELLOW)Check $(BUILD_DIR)/test-results.log for details$(NC)\n"; \
		exit 1; \
	fi

.PHONY: benchmark
benchmark: ## TEST: Run benchmarks
	@printf "$(BLUE)Running benchmarks...$(NC)\n"
	@go test -bench=. -benchmem -run=^$$ ./...
	@printf "$(GREEN)✓ Benchmarks completed$(NC)\n"

# Linting and code quality
.PHONY: lint
lint: ## TEST: Run golangci-lint
	@printf "$(BLUE)Running linter...$(NC)\n"
	@golangci-lint run --enable errcheck --timeout 5m
	@printf "$(GREEN)✓ Linting completed$(NC)\n"

.PHONY: lint-fix
lint-fix: ## TEST: Run golangci-lint with auto-fix
	@printf "$(BLUE)Running linter with auto-fix...$(NC)\n"
	@golangci-lint run --enable errcheck --fix --timeout 5m
	@printf "$(GREEN)✓ Linting with fixes completed$(NC)\n"

.PHONY: vet
vet: ## TEST: Run go vet
	@printf "$(BLUE)Running go vet...$(NC)\n"
	@go vet ./...
	@printf "$(GREEN)✓ Vet completed$(NC)\n"

.PHONY: fmt
fmt: ## TEST: Format code with gofmt
	@printf "$(BLUE)Formatting code...$(NC)\n"
	@gofmt -s -w $(GO_FILES)
	@printf "$(GREEN)✓ Code formatted$(NC)\n"

.PHONY: check
check: lint vet test ## TEST: Run all checks (lint, vet, test)
	@printf "$(GREEN)✓ All checks passed$(NC)\n"

# Running targets
.PHONY: run
run: ## RUN: Run the application with default settings
	@printf "$(BLUE)Running $(APP_NAME)...$(NC)\n"
	./$(APP_NAME)

.PHONY: run-all
run-all: ## RUN: Run with all protocol listeners enabled
	@printf "$(BLUE)Running $(APP_NAME) with all listeners...$(NC)\n"
	./$(APP_NAME) \
		--tls \
		--tcp \
		--grpc \
		--quic

.PHONY: run-debug
run-debug: ## RUN: Run with all listeners in debug log level
	@printf "$(BLUE)Running $(APP_NAME) with debug logging...$(NC)\n"
	./$(APP_NAME) \
		--tls \
		--tcp \
		--grpc \
		--quic \
		--log-level debug

.PHONY: debug
debug: ## RUN: Run with delve debugger (requires dlv)
	@command -v dlv >/dev/null 2>&1 || { printf "$(RED)Error: delve is not installed. Install it with: go install github.com/go-delve/delve/cmd/dlv@latest$(NC)\n"; exit 1; }
	@printf "$(BLUE)Starting $(APP_NAME) with delve debugger...$(NC)\n"
	@printf "$(YELLOW)Connect with: dlv connect :2345$(NC)\n"
	dlv debug cmd/echo-app/main.go -- \
		--tls \
		--tcp \
		--grpc \
		--quic \
		--log-level debug

.PHONY: run-docker
run-docker: ## RUN: Run the application in Docker
	@printf "$(BLUE)Running $(APP_NAME) in Docker...$(NC)\n"
	docker run -it --rm \
		-p 8080:8080 \
		-p 8443:8443 \
		-p 9090:9090 \
		-p 50051:50051 \
		-p 4433:4433/udp \
		-p 3000:3000 \
		-e ECHO_APP_TLS=true \
		-e ECHO_APP_TCP=true \
		-e ECHO_APP_GRPC=true \
		-e ECHO_APP_QUIC=true \
		-e ECHO_APP_MESSAGE="docker-test" \
		$(DOCKER_IMAGE)

# Docker targets
.PHONY: docker
docker: ## DOCKER: Build Docker image for current platform
	@printf "$(BLUE)Building Docker image...$(NC)\n"
	docker build \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(GIT_COMMIT) \
		--tag $(DOCKER_IMAGE):latest \
		--tag $(DOCKER_IMAGE):$(VERSION) \
		.
	@printf "$(GREEN)✓ Docker image built: $(DOCKER_IMAGE):$(VERSION)$(NC)\n"

.PHONY: docker-multi
docker-multi: ## DOCKER: Build multi-platform Docker image
	@printf "$(BLUE)Building multi-platform Docker image...$(NC)\n"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg VCS_REF=$(GIT_COMMIT) \
		--tag $(DOCKER_IMAGE):latest \
		--tag $(DOCKER_IMAGE):$(VERSION) \
		.
	@printf "$(GREEN)✓ Multi-platform Docker image built$(NC)\n"

.PHONY: docker-push
docker-push: ## DOCKER: Push Docker image to registry
	@printf "$(BLUE)Pushing Docker image...$(NC)\n"
	docker push $(DOCKER_IMAGE):latest
	docker push $(DOCKER_IMAGE):$(VERSION)
	@printf "$(GREEN)✓ Docker image pushed$(NC)\n"

# Maintenance targets
.PHONY: clean
clean: ## MAINT: Clean build artifacts and test cache
	@printf "$(BLUE)Cleaning build artifacts...$(NC)\n"
	@rm -f $(APP_NAME)
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE)
	@rm -f app.log build.log
	@go clean -testcache
	@printf "$(GREEN)✓ Cleanup completed$(NC)\n"

.PHONY: install
install: build ## MAINT: Install the application to GOPATH/bin
	@printf "$(BLUE)Installing $(APP_NAME)...$(NC)\n"
	@go install cmd/echo-app/main.go
	@printf "$(GREEN)✓ $(APP_NAME) installed to $(GOPATH)/bin$(NC)\n"

.PHONY: uninstall
uninstall: ## MAINT: Uninstall the application from GOPATH/bin
	@printf "$(BLUE)Uninstalling $(APP_NAME)...$(NC)\n"
	@rm -f $(GOPATH)/bin/$(APP_NAME)
	@printf "$(GREEN)✓ $(APP_NAME) uninstalled$(NC)\n"

.PHONY: update-deps
update-deps: ## MAINT: Update all dependencies
	@printf "$(BLUE)Updating dependencies...$(NC)\n"
	@go get -u ./...
	@go mod tidy
	@printf "$(GREEN)✓ Dependencies updated$(NC)\n"

.PHONY: verify
verify: ## MAINT: Verify dependencies
	@printf "$(BLUE)Verifying dependencies...$(NC)\n"
	@go mod verify
	@printf "$(GREEN)✓ Dependencies verified$(NC)\n"

.PHONY: info
info: ## MAINT: Show project information
	@printf "$(BLUE)Project Information$(NC)\n"
	@printf "$(BLUE)==================$(NC)\n"
	@echo "Name:       $(APP_NAME)"
	@echo "Version:    $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $$(go version)"
	@echo ""
	@printf "$(BLUE)Dependencies:$(NC)\n"
	@go list -m all | head -10
	@echo "... and $$(go list -m all | wc -l | tr -d ' ') more"
	@echo ""
	@printf "$(BLUE)Project Stats:$(NC)\n"
	@echo "Go files:   $$(find . -name '*.go' -not -path './vendor/*' | wc -l | tr -d ' ')"
	@echo "Lines:      $$(find . -name '*.go' -not -path './vendor/*' | xargs wc -l | tail -1 | awk '{print $$1}')"

# Utility targets
.PHONY: tools
tools: ## MAINT: Install required development tools
	@printf "$(BLUE)Installing development tools...$(NC)\n"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@printf "$(GREEN)✓ Development tools installed$(NC)\n"

.PHONY: pre-commit
pre-commit: fmt lint test ## MAINT: Run pre-commit checks
	@printf "$(GREEN)✓ Pre-commit checks passed$(NC)\n"

# Phony target to ensure these are always executed
.PHONY: all
all: clean deps build test ## MAINT: Clean, download deps, build, and test