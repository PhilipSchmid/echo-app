# echo-app

![Build and push Docker image](https://github.com/philipschmid/echo-app/actions/workflows/build.yaml/badge.svg) ![Go syntax and format check](https://github.com/philipschmid/echo-app/actions/workflows/lint.yaml/badge.svg) ![Go tests and app build](https://github.com/philipschmid/echo-app/actions/workflows/test.yaml/badge.svg)

The `echo-app` is a versatile Go application designed to echo back a JSON payload containing detailed information about incoming requests. It’s an invaluable tool for testing, debugging, and understanding network interactions across multiple protocols. The JSON response includes:

- **Timestamp**: When the request was received.
- **Source IP**: The IP address of the client making the request.
- **Hostname**: The name of the host running the application.
- **Listener Name**: The type of listener handling the request (e.g., HTTP, TLS, TCP, gRPC, QUIC).
- **HTTP Details** (for HTTP, TLS, QUIC listeners): HTTP version, method, endpoint, and optionally request headers.
- **gRPC Method** (for gRPC listener): The invoked gRPC method name.
- **Customizable Message**: An optional message to identify specific environments or configurations.
- **Node Name**: Useful in Kubernetes to identify the node hosting the pod.

### Supported Listeners
- **HTTP Listener**: Serves the JSON payload over HTTP.
- **TLS (HTTPS) Listener**: Uses an in-memory self-signed certificate for secure HTTPS communication.
- **QUIC Listener**: Supports HTTP/3 over QUIC with TLS encryption.
- **TCP Listener**: Provides the JSON payload over a raw TCP connection.
- **gRPC Listener**: Delivers the same details via a gRPC service.
- **Prometheus Metrics**: Exposes request metrics for monitoring.

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
Usage:
  --grpc                         Enable gRPC listener
  --grpc-port string             Port for the gRPC server (default "50051")
  --log-level string             Logging level (debug, info, warn, error) (default "info")
  --message string               Custom message to include in the response
  --metrics                      Enable Prometheus metrics endpoint
  --metrics-port string          Port for the Prometheus metrics server (default "3000")
  --node string                  Node name to include in the response
  --port string                  Port for the HTTP server (default "8080")
  --print-http-request-headers   Include HTTP request headers in the response
  --quic                         Enable QUIC listener
  --quic-port string             Port for the QUIC server (default "4433")
  --tcp                          Enable TCP listener
  --tcp-port string              Port for the TCP server (default "9090")
  --tls                          Enable TLS (HTTPS) support
  --tls-port string              Port for the TLS server (default "8443")
```

## Makefile Targets
Use these `make` targets to manage the application:

- `make`: Show the help message.
- `make build`: Build the Go application.
- `make vet`: Run `go vet` for code analysis.
- `make lint`: Run `golangci-lint` for static code checks.
- `make test`: Run unit tests with `go test`.
- `make docker`: Build a multi-arch Docker image.
- `make run`: Build and run the application locally.
- `make run-all`: Run the application with all listeners enabled.
- `make run-all-debug`: Run the application with all listeners and debug logging.
- `make test-all-endpoints`: Test all enabled endpoints.
- `make cleanup`: Remove build artifacts.

## Building the Application

### Local Build
To build the `echo-app` binary locally:

```bash
make build
```

### Container Image
To build a multi-architecture Docker image (for `amd64` and `arm64`):

```bash
make docker
```

## Running the Application

### Local
After building with `make build`, use one of these commands:

```bash
# Start only the HTTP listener:
make run
# Start all listeners:
make run-all
# Start all listeners with debug logging:
make run-all-debug
```

You can also pass flags directly:

```bash
./echo-app --tls --tcp --grpc --quic --log-level debug
```

### Standalone Container
Run the application in a Docker container with these examples:

```bash
# Basic HTTP listener
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:main
# With metrics exposed
docker run -it -p 8080:8080 -p 3000:3000 ghcr.io/philipschmid/echo-app:main
# With a custom message
docker run -it -p 8080:8080 -e ECHO_APP_MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:main
# With node name for Kubernetes
docker run -it -p 8080:8080 -e ECHO_APP_NODE="k8s-node-1" ghcr.io/philipschmid/echo-app:main
# Include HTTP request headers
docker run -it -p 8080:8080 -e ECHO_APP_PRINT_HTTP_REQUEST_HEADERS="true" ghcr.io/philipschmid/echo-app:main
# Enable TLS listener
docker run -it -p 8080:8080 -p 8443:8443 -e ECHO_APP_TLS="true" ghcr.io/philipschmid/echo-app:main
# Enable TCP listener
docker run -it -p 8080:8080 -p 9090:9090 -e ECHO_APP_TCP="true" ghcr.io/philipschmid/echo-app:main
# Enable gRPC listener
docker run -it -p 8080:8080 -p 50051:50051 -e ECHO_APP_GRPC="true" ghcr.io/philipschmid/echo-app:main
# Enable QUIC listener
docker run -it -p 8080:8080 -p 4433:4433/udp -e ECHO_APP_QUIC="true" ghcr.io/philipschmid/echo-app:main
# Disable Prometheus metrics
docker run -it -p 8080:8080 -e ECHO_APP_METRICS="false" ghcr.io/philipschmid/echo-app:main
```

## Testing

### HTTP Listener
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

With headers (`ECHO_APP_PRINT_HTTP_REQUEST_HEADERS="true"`):
```json
{
  "timestamp": "2024-08-06T12:10:07.743+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "HTTP",
  "headers": {
    "Accept": ["*/*"],
    "User-Agent": ["curl/8.10.0-DEV"]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### TLS (HTTPS) Listener
```bash
curl -sSk https://localhost:8443/ | jq
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T12:10:29.468+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "TLS",
  "headers": {
    "Accept": ["*/*"],
    "User-Agent": ["curl/8.10.0-DEV"]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### QUIC Listener (HTTP/3 & TLS)
Ensure you have a `curl` version with HTTP/3 support (e.g., [Cloudflare's curl](https://github.com/cloudflare/homebrew-cloudflare)):

```bash
curl --version | grep HTTP3
```

Test the QUIC listener:
```bash
curl -sSk --http3 https://localhost:4433/ | jq
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T12:11:13.158+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "QUIC",
  "headers": {
    "Accept": ["*/*"],
    "User-Agent": ["curl/8.10.0-DEV"]
  },
  "http_version": "HTTP/3.0",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### TCP Listener
```bash
nc localhost 9090 | jq
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T12:11:29.603+02:00",
  "message": "",
  "hostname": "demo-host",
  "listener": "TCP",
  "node": "",
  "source_ip": "192.168.65.1",
}
```

### gRPC Listener
```bash
grpcurl -plaintext -emit-defaults localhost:50051 echo.EchoService.Echo
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T12:11:39.15+02:00",
  "message": "",
  "sourceIp": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "gRPC",
  "grpcMethod": "/echo.EchoService/Echo"
}
```

### Prometheus Metrics
```bash
curl -sS http://localhost:3000/metrics
```

**Sample Output**:
```
# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{listener="HTTP",method="GET",endpoint="/"} 1
# HELP tcp_requests_total Total number of TCP requests
# TYPE tcp_requests_total counter
tcp_requests_total 1
# HELP grpc_requests_total Total number of gRPC requests
# TYPE grpc_requests_total counter
grpc_requests_total{method="/echo.EchoService/Echo"} 1
# HELP quic_requests_total Total number of QUIC requests
# TYPE quic_requests_total counter
quic_requests_total 1
```

## Kubernetes Deployment

Deploy `echo-app` in Kubernetes with all listeners enabled using these manifests:

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
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 1
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "app"
                  operator: In
                  values:
                  - echo-app
              topologyKey: "kubernetes.io/hostname"
      containers:
      - name: echo-app
        image: ghcr.io/philipschmid/echo-app:main
        ports:
        - name: http
          protocol: TCP
          containerPort: 8080
        - name: tls
          protocol: TCP
          containerPort: 8443
        - name: quic
          protocol: UDP
          containerPort: 4433
        - name: tcp
          protocol: TCP
          containerPort: 9090
        - name: grpc
          protocol: TCP
          containerPort: 50051
        - name: metrics
          protocol: TCP
          containerPort: 3000
        env:
        - name: ECHO_APP_MESSAGE
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_MESSAGE
        - name: ECHO_APP_PRINT_HTTP_REQUEST_HEADERS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_PRINT_HTTP_REQUEST_HEADERS
        - name: ECHO_APP_TLS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_TLS
        - name: ECHO_APP_QUIC
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_QUIC
        - name: ECHO_APP_GRPC
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_GRPC
        - name: ECHO_APP_TCP
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_TCP
        - name: ECHO_APP_METRICS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: ECHO_APP_METRICS
        - name: NODE
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
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
    protocol: TCP
    port: 8080
    targetPort: 8080
  - name: tls
    protocol: TCP
    port: 8443
    targetPort: 8443
  - name: quic
    protocol: UDP
    port: 4433
    targetPort: 4433
  - name: tcp
    protocol: TCP
    port: 9090
    targetPort: 9090
  - name: grpc
    protocol: TCP
    port: 50051
    targetPort: 50051
  - name: metrics
    protocol: TCP
    port: 3000
    targetPort: 3000
  type: ClusterIP
```

This deployment uses a `ConfigMap` to manage environment variables and the Downward API to inject the node name. The `Service` exposes all listener ports for cluster-internal access.

### Ingress Example
Expose the HTTP listener externally with an Ingress:

```yaml
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echo
spec:
  ingressClassName: cilium
  rules:
  - host: echo.<ip-of-ingress-lb-service>.sslip.io
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: echo-app-service
            port:
              number: 8080
```

Test the Ingress:
```bash
curl -sS http://echo.<ip-of-ingress-lb-service>.sslip.io | jq
```

**Sample Output**:
```json
{
  "timestamp": "2024-08-06T14:54:27.813Z",
  "message": "demo-env",
  "source_ip": "10.0.1.230",
  "hostname": "echo-app-deployment-699d7bf76f-k7k4h",
  "listener": "HTTP",
  "headers": {
    "Accept": ["*/*"],
    "User-Agent": ["curl/8.10.0-DEV"],
    "X-Envoy-External-Address": ["85.X.Y.Z"],
    "X-Forwarded-For": ["85.X.Y.Z"],
    "X-Forwarded-Proto": ["http"],
    "X-Request-Id": ["c32cb1aa-44d6-4484-8554-14a9984cff60"]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### Gateway API Example
For advanced routing with the Gateway API (e.g., using Cilium):

```yaml
# Infrastructure
---
apiVersion: v1
kind: Namespace
metadata:
  name: infra
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: echo-gw
  namespace: infra
spec:
  gatewayClassName: cilium
  listeners:
  - name: http
    protocol: HTTP
    port: 80
    allowedRoutes:
      namespaces:
        from: All
      kinds:
      - kind: HTTPRoute
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: tls-echo-gw
  namespace: infra
spec:
  gatewayClassName: cilium
  listeners:
  - name: tls
    protocol: TLS
    port: 443
    tls:
      mode: Passthrough
    allowedRoutes:
      namespaces:
        from: All
      kinds:
      - kind: TLSRoute
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: grpc-echo-gw
  namespace: infra
spec:
  gatewayClassName: cilium
  listeners:
  - name: grpc
    protocol: HTTP
    port: 50051
    allowedRoutes:
      namespaces:
        from: All
# Routes
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: echo
spec:
  parentRefs:
  - name: echo-gw
    namespace: infra
  hostnames:
  - echo.<ip-of-echo-gw-lb-service>.sslip.io
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 8080
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TLSRoute
metadata:
  name: tls-echo
spec:
  parentRefs:
  - name: tls-echo-gw
    namespace: infra
  hostnames:
  - tls-echo.<ip-of-tls-echo-gw-lb-service>.sslip.io
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 8443
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GRPCRoute
metadata:
  name: grpc-echo
spec:
  parentRefs:
  - name: grpc-echo-gw
    namespace: infra
  hostnames:
  - grpc-echo.<ip-of-grpc-echo-gw-lb-service>.sslip.io
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 50051
```

**Testing the Routes**:
- **HTTPRoute**:
  ```bash
  while true; do curl -sS http://echo.<ip-of-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
  ```
- **TLSRoute**:
  ```bash
  while true; do curl -sSk https://tls-echo.<ip-of-tls-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
  ```
- **GRPCRoute**:
  ```bash
  while true; do grpcurl -plaintext grpc-echo.<ip-of-grpc-echo-gw-lb-service>.sslip.io:50051 echo.EchoService/Echo; sleep 2; done
  ```