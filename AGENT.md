# Agent Instructions for echo-app

This document contains instructions for AI coding assistants working on the echo-app repository.

## Project Overview

echo-app is a multi-protocol echo server written in Go that supports:
- HTTP/HTTPS (with TLS)
- TCP
- gRPC
- QUIC (HTTP/3)
- Prometheus metrics
- Health and readiness endpoints

The application is designed for debugging, testing, and demonstrating various network protocols.

## Technology Stack

- **Language**: Go 1.25+
- **Build Tool**: Go modules
- **Testing**: Go standard testing, testify/assert
- **Linting**: golangci-lint
- **CI/CD**: GitHub Actions
- **Containerization**: Docker
- **Metrics**: Prometheus
- **Protocols**: HTTP/1.1, HTTP/2, HTTP/3 (QUIC), TCP, gRPC

## Development Workflow

### Code Quality Standards

1. **Testing Requirements**
   - All new features must include tests
   - Maintain or improve code coverage (current target: >50%)
   - All tests must pass with the race detector: `go test -race ./...`
   - Tests must be idempotent and avoid port conflicts
   - Use unique ports for each test server (18xxx, 19xxx, 13xxx ranges)

2. **Linting**
   - Code must pass `golangci-lint run --timeout 5m`
   - All linters are enabled, including errcheck
   - No unchecked error returns are allowed, even in tests
   - Use `_ =` prefix to explicitly ignore errors when appropriate

3. **Formatting**
   - All code must be formatted with `gofmt`
   - Struct fields must be properly aligned
   - Run `gofmt -w .` before committing

4. **Race Detection**
   - Protect shared state with appropriate synchronization primitives
   - Use `sync.Mutex` or `sync.RWMutex` for concurrent access
   - All concurrent code must pass `go test -race ./...`

### File Organization

```
.
├── cmd/echo-app/          # Main application entry point
├── internal/
│   ├── config/            # Configuration management
│   ├── handlers/          # Protocol handlers (HTTP, TCP, gRPC, QUIC)
│   ├── server/            # Server implementations
│   └── utils/             # Utility functions (cert generation, etc.)
├── proto/                 # Protocol buffer definitions
├── Dockerfile             # Container image definition
├── Makefile              # Build and test targets
└── README.md             # User documentation
```

### Testing Guidelines

1. **Test File Naming**: `*_test.go` in the same package
2. **Test Ports**: Use unique ports to avoid conflicts
   - HTTP tests: 18080-18099
   - TCP tests: 19090-19099
   - Metrics tests: 13000-13009
3. **Concurrency**: Use `sync.WaitGroup` and channels for synchronization
4. **Cleanup**: Always use `defer` for cleanup (cancel contexts, close connections)
5. **Error Handling**: All error returns must be checked or explicitly ignored with `_ =`

### Common Patterns

#### Starting Servers in Tests
```go
go func() { _ = server.Start(ctx) }()  // Explicitly ignore error
time.Sleep(100 * time.Millisecond)     // Wait for server to start
```

#### Shutting Down Servers
```go
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
defer shutdownCancel()
_ = server.Shutdown(shutdownCtx)  // Or use assert.NoError() if critical
```

#### Error Handling in Tests
```go
// For critical operations
require.NoError(t, err)

// For explicitly ignored errors
_ = conn.Close()
_, _ = io.Copy(io.Discard, resp.Body)

// For deferred cleanup
defer func() { _ = resp.Body.Close() }()
```

## Git Commit Guidelines

### Commit Message Format

Follow the Conventional Commits specification:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks
- `ci`: CI/CD changes
- `deps`: Dependency updates

**Examples:**
```
feat: add configurable request body size limit

Add configurable maximum request body size limit to prevent resource
exhaustion attacks via large request payloads. Default is 10MB.

Configuration:
- Environment variable: ECHO_APP_MAX_REQUEST_SIZE
- Command-line flag: --max-request-size
- Default: 10485760 bytes (10MB)
```

```
fix: resolve data race conditions in TCP server

Fix critical data race conditions detected by the race detector in the
TCP server implementation. Add proper synchronization for concurrent
access to shared fields.

Changes:
- Add RWMutex to protect concurrent access to listener and ctx
- Use mutex in Start() when setting listener and context
- Use mutex in Shutdown() when accessing listener
```

### Commit Message Rules

**IMPORTANT**:
- **NEVER** mention AI, Claude Code, or similar in commit messages
- **NEVER** add "Co-Authored-By: Claude" or similar footers
- **NEVER** add "Generated with Claude Code" or similar notices
- Write commit messages as if written by a human developer
- Focus on the "why" and "what" of the change, not the "how"
- Keep commits focused and atomic
- Write clear, concise commit messages in imperative mood

### Good Commit Messages
```
fix: add panic recovery to HTTP handler

Add panic recovery mechanism to prevent crashes from unexpected panics.
This ensures that a single malformed request cannot crash the entire
HTTP/TLS listener.

- Add defer recover() at the start of handler
- Log panic details with context
- Record panic as error metric
- Return HTTP 500 to client on panic
```

### Bad Commit Messages (DO NOT USE)
```
fix: add panic recovery to HTTP handler

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

## GitHub Actions CI

The CI pipeline runs on every push and pull request:

1. **Linting**: `golangci-lint run`
2. **Formatting**: `gofmt -l .`
3. **Tests**: `go test -race ./...`
4. **Build**: `go build ./...`

All checks must pass before merging.

### Development Workflow Best Practices

**Before committing code:**

1. **Clear linter cache** to ensure fresh results matching CI behavior
2. **Run all checks locally** using the same commands as CI
3. **Verify all checks pass** before pushing commits

**When making code changes:**

1. **Search comprehensively** - Use grep/search to find all instances of patterns you're modifying
2. **Fix systematically** - Address all occurrences at once, not incrementally
3. **Test thoroughly** - Run tests with race detector after every significant change
4. **Validate locally** - Ensure linter returns success (exit code 0) before committing

This workflow prevents CI failures by catching issues locally before they reach the remote repository.

## Security Considerations

1. **Input Validation**: Validate all user inputs
2. **Resource Limits**: Enforce connection limits, request size limits, timeouts
3. **TLS**: Use strong TLS configurations, generate proper certificates
4. **Panic Recovery**: Add panic recovery to all server handlers
5. **Metrics**: Don't create unlimited time series (normalize endpoints)

## Common Tasks

### Adding a New Feature

1. Create tests first (TDD approach)
2. Implement the feature
3. Ensure tests pass with race detector
4. Run linter and fix all issues
5. Update documentation if needed
6. Commit with conventional commit message (no AI mentions)

### Fixing a Bug

1. Write a failing test that reproduces the bug
2. Fix the bug
3. Ensure test passes
4. Run full test suite with race detector
5. Commit with fix: prefix (no AI mentions)

### Updating Dependencies

1. Run `go get -u ./...` or update specific packages
2. Run `go mod tidy`
3. Run full test suite
4. Check for breaking changes
5. Commit with deps: prefix

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -cover ./...

# Run specific test
go test -run TestHTTPServer_StartAndStop ./internal/server

# Run linter
golangci-lint run --timeout 5m

# Format code
gofmt -w .
```

## Configuration

The application uses environment variables and command-line flags:

- `ECHO_APP_MESSAGE`: Custom message to echo back
- `ECHO_APP_NODE`: Node identifier
- `ECHO_APP_HTTP_PORT`: HTTP port (default: 8080)
- `ECHO_APP_TLS_PORT`: TLS port (default: 8443)
- `ECHO_APP_TCP_PORT`: TCP port (default: 9090)
- `ECHO_APP_GRPC_PORT`: gRPC port (default: 50051)
- `ECHO_APP_QUIC_PORT`: QUIC port (default: 4433)
- `ECHO_APP_METRICS_PORT`: Metrics port (default: 3000)
- `ECHO_APP_MAX_REQUEST_SIZE`: Max request body size (default: 10MB)
- `ECHO_APP_LOG_LEVEL`: Log level (debug, info, warn, error)

## Additional Notes

- The codebase uses the internal/ directory to prevent external imports
- Protocol buffers are in proto/ and compiled to internal/handlers/
- TLS certificates are auto-generated using self-signed certs
- All servers support graceful shutdown with context cancellation
- Connection limits are enforced to prevent resource exhaustion
- Metrics follow Prometheus naming conventions
