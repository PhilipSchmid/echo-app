# echo-app

![Build and push Docker image](https://github.com/philipschmid/echo-app/actions/workflows/build.yaml/badge.svg) ![Go syntax and format check](https://github.com/philipschmid/echo-app/actions/workflows/lint.yaml/badge.svg) ![Go tests and app build](https://github.com/philipschmid/echo-app/actions/workflows/test.yaml/badge.svg)

This is a simple Go application that responds with a JSON payload containing various details. The JSON response includes:

- Timestamp
- Source IP
- Hostname
- Listener name
- HTTP version, HTTP method, HTTP endpoint, and HTTP request headers (HTTP, TLS, and QUIC listeners only)
- gRPC method (gRPC listener only)
- Optionally, a customizable message and the (Kubernetes) node name.

The application supports multiple listeners and functionalities:

- **HTTP Listener**: Responds with the JSON payload over HTTP.
- **TLS (HTTPS) Listener**:
  - Generates an in-memory self-signed TLS certificate.
  - Allows secure communication over a dedicated HTTPS port.
  - Returns the same JSON message over a TLS-encrypted HTTP connection.
- **QUIC Listener**:
  - Generates an in-memory self-signed TLS certificate.
  - Returns the same JSON message over a TLS-encrypted HTTP/3 over QUIC connection.
- **TCP Listener**: Serves the same JSON message over a TCP connection (minus the request headers).
- **gRPC Listener**: Provides the same information using gRPC (minus the request headers).
- **Prometheus Metrics**: Exposes metrics about the listeners/endpoints.

## Configuration Options

### Environment Variables
- `ECHO_APP_MESSAGE`: A customizable message to be returned in the JSON response. If not set, no message will be displayed.
- `ECHO_APP_NODE`: The name of the node where the app is running. This is typically used in a Kubernetes environment.
- `ECHO_APP_PORT`: The port number on which the HTTP server listens. Default is `8080` (TCP).
- `ECHO_APP_PRINT_HTTP_REQUEST_HEADERS`: Set to `true` to include HTTP request headers in the JSON response. By default, headers are not included.
- `ECHO_APP_TLS`: Set to `true` to enable TLS (HTTPS) support. By default, TLS is disabled.
- `ECHO_APP_TLS_PORT`: The port number on which the TLS server listens. Default is `8443` (TCP).
- `ECHO_APP_TCP`: Set to `true` to enable the TCP listener. By default, TCP is disabled.
- `ECHO_APP_TCP_PORT`: The port number on which the TCP server listens. Default is `9090` (TCP).
- `ECHO_APP_GRPC`: Set to `true` to enable the gRPC listener. By default, gRPC is disabled.
- `ECHO_APP_GRPC_PORT`: The port number on which the gRPC server listens. Default is `50051` (TCP).
- `ECHO_APP_QUIC`: Set to `true` to enable the QUIC listener. By default, QUIC is disabled.
- `ECHO_APP_QUIC_PORT`: The port number on which the QUIC server listens. Default is `4433` (UDP).
- `ECHO_APP_METRICS`: Set to `true` to enable Prometheus metrics endpoint. By default, metrics are enabled.
- `ECHO_APP_METRICS_PORT`: The port number on which the Prometheus metrics server listens. Default is `3000` (TCP).
- `ECHO_APP_LOG_LEVEL`: Set the logging level (`debug`, `info`, `warn`, `error`). Default is `info`.

### Commandline Flags
```bash
./echo-app --help
echo-app: A simple Go application that responds with a JSON payload containing various details.

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
- `make`: Show the help message.
- `make build`: Build the Go application.
- `make vet`: Run `go vet` to examine Go source code and report suspicious constructs.
- `make lint`: Run `golangci-lint` to perform static code analysis.
- `make test`: Run the tests using `go test`.
- `make docker`: Build the Docker image using `docker buildx build`.
- `make run`: Build and run the Go application.

## Building the Application

### Local Build
Building the `echo-app` binary:

```bash
make build
```

### Container Image
Building the Docker image:

```bash
make docker
```

This will build a multi-arch Docker image for both `amd64` and `arm64` platforms.

## Running the Application

### Local
Use one of the following commands to run the application locally after building it with `make build`:

```bash
# Start only the HTTP listener (default):
make run
# Alternatively, to enable all listeners:
make run-all
```

### Standalone Container
Examples how to run the application within a standalone container:

```bash
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:main
# Optionally with exposed metrics endpoint
docker run -it -p 8080:8080 -p 3000:3000 ghcr.io/philipschmid/echo-app:main
# Optionally with a customized message:
docker run -it -p 8080:8080 -e ECHO_APP_MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:main
# Optionally with a node name:
docker run -it -p 8080:8080 -e ECHO_APP_NODE="k8s-node-1" ghcr.io/philipschmid/echo-app:main
# Optionally include HTTP request headers in the response:
docker run -it -p 8080:8080 -e ECHO_APP_PRINT_HTTP_REQUEST_HEADERS="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable TLS:
docker run -it -p 8080:8080 -p 8443:8443 -e ECHO_APP_TLS="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable TCP:
docker run -it -p 8080:8080 -p 9090:9090 -e ECHO_APP_TCP="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable gRPC:
docker run -it -p 8080:8080 -p 50051:50051 -e ECHO_APP_GRPC="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable QUIC:
docker run -it -p 8080:8080 -p 4433:4433/udp -e ECHO_APP_QUIC="true" ghcr.io/philipschmid/echo-app:main
# Optionally disable Prometheus metrics:
docker run -it -p 8080:8080 -e ECHO_APP_METRICS="false" ghcr.io/philipschmid/echo-app:main
```

## Testing

### HTTP Listener

```bash
curl -sS http://localhost:8080/ | jq
```

You should see a similar output like this:

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

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:

```json
{
  "timestamp": "2024-08-06T12:10:07.743+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "HTTP",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### TLS (HTTPS) Listener

If `TLS` is enabled, you can test the HTTPS listener:

```bash
curl -sSk https://localhost:8443/ | jq
```

You should see a similar output like this:

```json
{
  "timestamp": "2024-08-06T12:10:29.468+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "TLS",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### QUIC Listener (HTTP/3 & TLS)

To test the QUIC (HTTP/3) listener, you need to use a `curl` version which supports it. For example, the one [from CloudFlare](https://github.com/cloudflare/homebrew-cloudflare). Check out [this guide](https://dev.to/gjrdiesel/installing-curl-with-http3-on-macos-2di2) to learn how to install it on macOS.

Ensure your `curl` has built-in `HTTP3` support:

```bash
$ curl --version | grep HTTP3
Features: alt-svc AsynchDNS brotli GSS-API HSTS HTTP2 HTTP3 HTTPS-proxy IDN IPv6 Kerberos Largefile libz NTLM SPNEGO SSL threadsafe UnixSockets zstd
```

Testing the QUIC listener:

```bash
curl -sSk --http3 https://localhost:4433/ | jq
```

You should see a similar output like this:

```json
{
  "timestamp": "2024-08-06T12:11:13.158+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "QUIC",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ]
  },
  "http_version": "HTTP/3.0",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### TCP Listener

To test the TCP listener using netcat:

```bash
nc localhost 9090 | jq
```

You should see a similar output like this:

```json
{
  "timestamp": "2024-08-06T12:11:29.603+02:00",
  "source_ip": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "TCP"
}
```

### GRPC Listener

To test the gRPC listener, you can use a gRPC client like `grpcurl`:

```bash
grpcurl -plaintext localhost:50051 echo.EchoService/Echo
```

You should see a similar output like this:

```json
{
  "timestamp": "2024-08-06T12:11:39.15+02:00",
  "sourceIp": "192.168.65.1",
  "hostname": "demo-host",
  "listener": "gRPC",
  "grpcMethod": "/echo.EchoService/Echo"
}
```

### Prometheus Metrics

If `METRICS` is enabled (default), you can access the Prometheus metrics endpoint:

```bash
curl -sS http://localhost:3000/metrics
```

You should see a similar output like this:

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

For example, if you're running a cluster with Cilium installed like this: https://gist.github.com/PhilipSchmid/bf4e4d2382678959f29f6e0d7b9b4725

Apply the following manifests to deploy the echo-app with the `NODE` environment variable set to the name of the Kubernetes node using the Downward API:

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
  replicas: 3 # Adjust the number of replicas as needed
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

Shell 2 (client):

```bash
kubectl run netshoot -it --rm --image=nicolaka/netshoot --restart=Never -q -- curl -sS http://echo-app-service:8080 | jq
```

You should see a similar client output like this:

```json
{
  "timestamp": "2024-08-06T14:52:21.129Z",
  "message": "demo-env",
  "source_ip": "10.0.0.163",
  "hostname": "echo-app-deployment-699d7bf76f-mn8qs",
  "listener": "HTTP",
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:

```json
{
  "timestamp": "2024-08-06T14:52:35.215Z",
  "message": "demo-env",
  "source_ip": "10.0.0.63",
  "hostname": "echo-app-deployment-699d7bf76f-mn8qs",
  "listener": "HTTP",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.7.1"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

### Ingress Example

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

```bash
curl -sS http://echo.52.167.255.246.sslip.io | jq
```

You should see a similar client output like this:
```json
{
  "timestamp": "2024-08-06T14:54:27.813Z",
  "message": "demo-env",
  "source_ip": "10.0.1.230",
  "hostname": "echo-app-deployment-699d7bf76f-k7k4h",
  "listener": "HTTP",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ],
    "X-Envoy-External-Address": [
      "85.X.Y.Z"
    ],
    "X-Forwarded-For": [
      "85.X.Y.Z"
    ],
    "X-Forwarded-Proto": [
      "http"
    ],
    "X-Request-Id": [
      "c32cb1aa-44d6-4484-8554-14a9984cff60"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

Side note: Some headers are automatically added by Cilium Ingress.

### Gateway API Example

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
  - grpc-echo.<ip-of-tls-echo-gw-lb-service>.sslip.io
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 50051
```

Side note: Cilium does not yet support `TCPRoute` or `UDPRoutes`. See https://github.com/cilium/cilium/issues/21929 for more details.

Testing `HTTPRoute`:

```bash
$ while true; do curl -sS http://echo.<ip-of-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
{
  "timestamp": "2024-08-06T15:14:21.299Z",
  "message": "demo-env",
  "source_ip": "10.0.1.230",
  "hostname": "echo-app-deployment-699d7bf76f-mtmt7",
  "listener": "HTTP",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ],
    "X-Envoy-External-Address": [
      "85.X.Y.Z"
    ],
    "X-Forwarded-For": [
      "85.X.Y.Z"
    ],
    "X-Forwarded-Proto": [
      "http"
    ],
    "X-Request-Id": [
      "107e4dc4-548b-4d02-8170-77feffe8552e"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

Testing `TLSRoute`:

```bash
$ while true; do curl -sSk https://tls-echo.<ip-of-tls-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
{
  "timestamp": "2024-08-06T15:15:40.74Z",
  "message": "demo-env",
  "source_ip": "10.0.0.213",
  "hostname": "echo-app-deployment-699d7bf76f-k7k4h",
  "listener": "TLS",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.10.0-DEV"
    ]
  },
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/"
}
```

Testing `GRPCRoute`:

```bash
$ while true; do grpcurl -plaintext <ip-of-grpc-echo-gw-lb-service>.sslip.io:50051 echo.EchoService/Echo; sleep 2; done
{
  "timestamp": "2024-08-06T15:26:30.638Z",
  "message": "demo-env",
  "sourceIp": "10.0.2.56",
  "hostname": "echo-app-deployment-699d7bf76f-mtmt7",
  "listener": "gRPC",
  "grpcMethod": "/echo.EchoService/Echo"
}
```