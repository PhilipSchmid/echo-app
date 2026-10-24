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
- **TCP Listener**: Sends one JSON metadata response and closes the connection.
- **gRPC Listener**: Delivers the same details via a gRPC service with reflection support.
- **Prometheus Metrics**: Exposes unified request metrics for monitoring.

## Configuration Options

### Environment Variables
Configure the application using these environment variables:

- `ECHO_APP_MESSAGE`: A customizable message included in the response. If unset, no message is included.
- `ECHO_APP_NODE`: The name of the node where the app is running (e.g., for Kubernetes).
- `ECHO_APP_HTTP_PORT`: Port for the HTTP server (default: `8080` TCP).
- `ECHO_APP_PRINT_HTTP_REQUEST_HEADERS`: Set to `true` to include HTTP request headers in the response.
- `ECHO_APP_UNSAFE_HEADERS`: Set to `true` to include sensitive header and gRPC metadata values in diagnostics (default: `false`).
- `ECHO_APP_H2C`: Set to `true` to enable HTTP/2 cleartext (h2c) on the HTTP listener.
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
- `ECHO_APP_MAX_REQUEST_SIZE`: Maximum request body size in bytes (default: `10485760` - 10MB).
- `ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE`: Optional external readiness probe type: `none`, `http`, `tcp`, or `icmp` (default: `none`).
- `ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET`: External readiness target, such as `https://api.example.com/ready`, `db.example.com:5432`, or `10.0.0.10`.
- `ECHO_APP_EXTERNAL_READINESS_PROBE_INTERVAL`: How often the background readiness controller checks the target (default: `10s`).
- `ECHO_APP_EXTERNAL_READINESS_PROBE_TIMEOUT`: Per-check timeout before the app marks itself not ready (default: `2s`).
- `ECHO_APP_EXTERNAL_READINESS_HTTP_METHOD`: HTTP method for external HTTP readiness probes (default: `GET`).
- `ECHO_APP_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS`: Expected HTTP status code for external HTTP readiness probes (default: `200`).

### Command-Line Flags
Run `./echo-app --help` to see all available flags:

```bash
Usage of ./echo-app:
      --external-readiness-http-expected-status int
                                     Expected HTTP status for external readiness HTTP probes (default 200)
      --external-readiness-http-method string
                                     HTTP method for external readiness HTTP probes (default "GET")
      --external-readiness-probe-interval duration
                                     External readiness probe interval (default 10s)
      --external-readiness-probe-target string
                                     External readiness probe target URL, host:port, or host/IP
      --external-readiness-probe-timeout duration
                                     External readiness probe timeout (default 2s)
      --external-readiness-probe-type string
                                     External readiness probe type: none, http, tcp, or icmp (default "none")
      --grpc                         Enable gRPC server
      --grpc-port string             gRPC server port (default "50051")
      --http-port string             HTTP server port (default "8080")
      --log-level string             Log level (debug, info, warn, error) (default "info")
      --max-request-size int         Maximum request body size in bytes (default 10485760)
      --message string               Custom message
      --metrics                      Enable metrics server (default true)
      --metrics-port string          Metrics server port (default "3000")
      --node string                  Node name
      --print-http-request-headers   Print HTTP request headers
      --quic                         Enable QUIC server
      --quic-port string             QUIC server port (default "4433")
      --tcp                          Enable TCP server
      --tcp-port string              TCP server port (default "9090")
      --h2c                          Enable HTTP/2 cleartext (h2c) on the HTTP listener
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
- `make test-integration` - Run real protocol integration tests, including HTTP/3
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
- QUIC/HTTP3 using a native Go client
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
# Health endpoint (liveness): returns 200 only when the echo app process is healthy
curl -s http://localhost:3000/health
# Returns: healthy

# Readiness endpoint: returns 200 only when the echo app is ready and any configured external readiness probe is passing
curl -s http://localhost:3000/ready
# Returns: ready

# Kubernetes probes should use the dedicated metrics/admin listener (default port 3000), not the echo traffic listeners.

# Prometheus metrics
curl -s http://localhost:3000/metrics | grep echo_app
```

#### External Readiness Probe Examples
The external readiness probe is optional. When enabled, the app keeps checking the configured target in the background and `/ready` returns `503` until the target succeeds within the configured timeout.

```bash
# HTTP readiness dependency: expect a 200 from the upstream readiness URL
ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=http \
ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=https://upstream.example.com/ready \
ECHO_APP_EXTERNAL_READINESS_PROBE_TIMEOUT=2s \
ECHO_APP_EXTERNAL_READINESS_PROBE_INTERVAL=10s \
./echo-app

# TCP readiness dependency: require a connectable host:port
ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=tcp \
ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=db.example.com:5432 \
./echo-app

# ICMP readiness dependency: require one successful echo reply within the timeout
ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=icmp \
ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=10.0.0.10 \
./echo-app
```

The ICMP probe runs in-process and does not require a `ping` binary. Ordinary
operation runs as user 65532 without capabilities. Raw ICMP is an explicit
exception: on Docker, adding `NET_RAW` alone does not give this non-root user
access to raw sockets. The following mode was verified with a UID 0 override
while retaining a read-only filesystem and only the required capability:

```bash
docker run --rm --user 0:0 --read-only \
  --cap-drop=ALL --cap-add=NET_RAW --security-opt=no-new-privileges \
  -p 127.0.0.1:3000:3000 \
  -e ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=icmp \
  -e ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=127.0.0.1 \
  ghcr.io/philipschmid/echo-app:main
```

Use HTTP or TCP readiness probes to retain the non-root defaults. The Kubernetes
example below deliberately drops all capabilities; raw ICMP requires an
explicitly adjusted security context instead.

### Safety and protocol behavior

The default response describes the request and the receiving server. Request
bodies are consumed within `ECHO_APP_MAX_REQUEST_SIZE` and discarded; oversized
bodies return `413`. TCP sends a single JSON document followed by EOF, and gRPC
accepts an empty request. Neither currently echoes application payloads.

Header diagnostics redact credentials, cookies, and secret/token/key fields by
default. `--unsafe-headers` deliberately disables this redaction for response
headers and debug metadata logs; use it only with synthetic credentials.
Diagnostic HTTP responses use `Cache-Control: no-store`.

Readiness requires every configured listener to bind and any configured external
probe to pass. A listener failure terminates the process. Liveness does not depend
on an external service. Invalid or incomplete probe settings fail at startup.

Keep the metrics/admin listener private and apply network policies to diagnostic
listeners. Pin an image version or digest for repeatable deployments.

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
  # Optional: mark this pod not-ready when a required upstream stops responding.
  # ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE: "http"
  # ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET: "https://upstream.example.com/ready"
  # ECHO_APP_EXTERNAL_READINESS_HTTP_METHOD: "GET"
  # ECHO_APP_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS: "200"
  # ECHO_APP_EXTERNAL_READINESS_PROBE_TIMEOUT: "2s"
  # ECHO_APP_EXTERNAL_READINESS_PROBE_INTERVAL: "10s"
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
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: echo-app
        image: ghcr.io/philipschmid/echo-app:main
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop: ["ALL"]
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
