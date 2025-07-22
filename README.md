# echo-app

![CI](https://github.com/philipschmid/echo-app/actions/workflows/ci.yaml/badge.svg) ![Docker Build](https://github.com/philipschmid/echo-app/actions/workflows/docker.yaml/badge.svg) ![Release](https://github.com/philipschmid/echo-app/actions/workflows/release.yaml/badge.svg)

The `echo-app` is a versatile Go application designed to echo back a JSON payload containing detailed information about incoming requests. It's an invaluable tool for testing, debugging, and understanding network interactions across multiple protocols. The JSON response includes:

- **Timestamp**: When the request was received.
- **Source IP**: The IP address of the client making the request.
- **Hostname**: The name of the host running the application.
- **Listener Name**: The type of listener handling the request (e.g., HTTP, TLS, TCP, gRPC, QUIC).
- **HTTP Details** (for HTTP, TLS, QUIC listeners): HTTP version, method, endpoint, and optionally request headers.
- **gRPC Method** (for gRPC listener): The invoked gRPC method name.
- **Customizable Message**: An optional message to identify specific environments or configurations.
- **Node Name**: Useful in Kubernetes to identify the node hosting the pod.

## Key Features

- **HTTP Listener**: Serves the JSON payload over HTTP.
- **TLS (HTTPS) Listener**: Uses an in-memory self-signed certificate for secure HTTPS communication.
- **QUIC Listener**: Supports HTTP/3 over QUIC with TLS encryption.
- **TCP Listener**: Provides the JSON payload over a raw TCP connection with connection pooling.
- **gRPC Listener**: Delivers the same details via a gRPC service with reflection support.
- **Prometheus Metrics**: Exposes unified request metrics for monitoring.

## Configuration Options

### Environment Variables
Configure the application using these environment variables:

- `ECHO_APP_MESSAGE`: A customizable message included in the response. If unset, no message is included.
- `ECHO_APP_NODE`: The name of the node where the app is running (e.g., for Kubernetes).
- `ECHO_APP_PORT`: Port for the HTTP server (default: `8080` TCP).
- `ECHO_APP_PRINT_HTTP_REQUEST_HEADERS`: Set to `true` to include HTTP request headers in the response.
- `ECHO_APP_TLS`: Set to `true` to enable the TLS (HTTPS) listener.
- `ECHO_APP_TLS_PORT`: Port for the TLS server (default: `8443` TCP).
- `ECHO_APP_TCP`: Set to `true` to enable the TCP listener.
- `ECHO_APP_TCP_PORT`: Port for the TCP server (default: `9090` TCP).
- `ECHO_APP_GRPC`: Set to `true` to enable the gRPC listener.
- `ECHO_APP_GRPC_PORT`: Port for the gRPC server (default: `50051` TCP).
- `ECHO_APP_QUIC`: Set to `true` to enable the QUIC listener.
- `ECHO_APP_QUIC_PORT`: Port for the QUIC server (default: `4433` UDP).
- `ECHO_APP_METRICS`: Set to `true` to enable the Prometheus metrics endpoint (default: `true`).
- `ECHO_APP_METRICS_PORT`: Port for the metrics server (default: `3000` TCP).
- `ECHO_APP_LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`; default: `info`).

### Command-Line Flags
Run `./echo-app --help` to see all available flags:

```bash
Usage of ./echo-app:
      --grpc                         Enable gRPC server
      --grpc-port string             gRPC server port (default "50051")
      --http-port string             HTTP server port (default "8080")
      --log-level string             Log level (debug, info, warn, error) (default "info")
      --message string               Custom message
      --metrics                      Enable metrics server (default true)
      --metrics-port string          Metrics server port (default "3000")
      --node string                  Node name
      --print-http-request-headers   Print HTTP request headers
      --quic                         Enable QUIC server
      --quic-port string             QUIC server port (default "4433")
      --tcp                          Enable TCP server
      --tcp-port string              TCP server port (default "9090")
      --tls                          Enable TLS server
      --tls-port string              TLS server port (default "8443")
```

## Quick Start

### Using Make
```bash
# Show all available commands
make help

# Quick build and run
make build-quick && make run

# Run with all protocols enabled
make run-all

# Run integration tests
make test-integration
```

### Using Docker
```bash
# Run with all protocols enabled
docker run -it --rm \
  -p 8080:8080 -p 8443:8443 -p 9090:9090 \
  -p 50051:50051 -p 4433:4433/udp -p 3000:3000 \
  -e ECHO_APP_TLS=true \
  -e ECHO_APP_TCP=true \
  -e ECHO_APP_GRPC=true \
  -e ECHO_APP_QUIC=true \
  ghcr.io/philipschmid/echo-app:main
```

## Makefile Targets

The Makefile has been significantly enhanced with categorized targets and color-coded output:

### Development
- `make dev` - Run with file watching (requires entr)
- `make proto` - Generate protobuf files
- `make deps` - Download and verify dependencies
- `make tidy` - Tidy and vendor dependencies

### Building
- `make build` - Build with all checks (lint, vet, test)
- `make build-quick` - Quick build without checks
- `make build-all` - Build for all platforms (Linux, macOS, Windows)

### Testing
- `make test` - Run unit tests with coverage
- `make test-coverage` - Generate detailed HTML coverage report
- `make test-short` - Run only short tests
- `make test-integration` - Run comprehensive integration tests
- `make benchmark` - Run performance benchmarks
- `make lint` - Run golangci-lint
- `make lint-fix` - Run golangci-lint with auto-fix
- `make check` - Run all checks (lint, vet, test)

### Running
- `make run` - Run with default settings
- `make run-all` - Run with all protocol listeners
- `make run-debug` - Run with debug logging
- `make run-docker` - Run in Docker container

### Docker
- `make docker` - Build Docker image for current platform
- `make docker-multi` - Build multi-platform Docker image
- `make docker-push` - Push Docker image to registry

### Maintenance
- `make clean` - Clean build artifacts and test cache
- `make install` - Install to GOPATH/bin
- `make info` - Show project information and statistics
- `make tools` - Install required development tools
- `make pre-commit` - Run pre-commit checks

## Building the Application

### Local Build
```bash
# Full build with all checks
make build

# Quick build for development
make build-quick

# Cross-platform builds
make build-all
```

### Container Image
```bash
# Build for current platform
make docker

# Build multi-architecture image (amd64 and arm64)
make docker-multi
```

## Running the Application

### Local Execution
```bash
# Start only the HTTP listener
make run

# Start all listeners
make run-all

# Start with debug logging
make run-debug

# Or run directly with specific flags
./echo-app --tls --tcp --grpc --quic --log-level debug
```

### Development Mode
```bash
# Run with file watching (auto-restart on changes)
make dev
```

## Testing

### Unit Tests
```bash
# Run all tests
make test

# Generate coverage report
make test-coverage

# Run benchmarks
make benchmark
```

### Integration Tests
```bash
# Run comprehensive integration tests
make test-integration
```

This will test:
- HTTP endpoint with various paths
- TLS/HTTPS endpoint
- TCP raw connections
- gRPC service calls
- QUIC/HTTP3 (if curl supports it)
- Prometheus metrics endpoint
- Health and readiness checks
- Graceful shutdown behavior

### Manual Testing Examples

#### HTTP Listener
```bash
curl -sS http://localhost:8080/ | jq
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T12:09:46.174+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "HTTP",
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

#### TLS (HTTPS) Listener
```bash
curl -sSk https://localhost:8443/ | jq
```

#### TCP Listener
```bash
echo "test" | nc localhost 9090 | jq
```

#### gRPC Listener
```bash
grpcurl -plaintext -emit-defaults localhost:50051 echo.EchoService.Echo
```

#### Health Checks
```bash
# Health endpoint
curl -s http://localhost:3000/health
# Returns: OK

# Readiness endpoint
curl -s http://localhost:3000/ready
# Returns: Ready

# Prometheus metrics
curl -s http://localhost:3000/metrics | grep echo_app
```

### Unified Metrics

The application now exposes unified Prometheus metrics:

```
# Request metrics
echo_app_requests_total{listener="HTTP",method="GET",endpoint="/"}
echo_app_request_duration_seconds{listener="HTTP",method="GET",endpoint="/"}

# Error metrics
echo_app_errors_total{listener="HTTP",error_type="marshal_error"}

# Connection metrics (for TCP)
echo_app_active_connections{listener="TCP"}
```

## Kubernetes Deployment

Deploy `echo-app` in Kubernetes with all listeners enabled:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-app-config
data:
  ECHO_APP_MESSAGE: "demo-env"
  ECHO_APP_PRINT_HTTP_REQUEST_HEADERS: "true"
  ECHO_APP_TLS: "true"
  ECHO_APP_QUIC: "true"
  ECHO_APP_GRPC: "true"
  ECHO_APP_TCP: "true"
  ECHO_APP_METRICS: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-app-deployment
  labels:
    app: echo-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: echo-app
  template:
    metadata:
      labels:
        app: echo-app
    spec:
      containers:
      - name: echo-app
        image: ghcr.io/philipschmid/echo-app:main
        ports:
        - name: http
          containerPort: 8080
        - name: tls
          containerPort: 8443
        - name: quic
          containerPort: 4433
          protocol: UDP
        - name: tcp
          containerPort: 9090
        - name: grpc
          containerPort: 50051
        - name: metrics
          containerPort: 3000
        envFrom:
        - configMapRef:
            name: echo-app-config
        env:
        - name: ECHO_APP_NODE
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        livenessProbe:
          httpGet:
            path: /health
            port: metrics
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: metrics
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: echo-app-service
spec:
  selector:
    app: echo-app
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: tls
    port: 8443
    targetPort: 8443
  - name: quic
    port: 4433
    targetPort: 4433
    protocol: UDP
  - name: tcp
    port: 9090
    targetPort: 9090
  - name: grpc
    port: 50051
    targetPort: 50051
  - name: metrics
    port: 3000
    targetPort: 3000
  type: ClusterIP
```

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.