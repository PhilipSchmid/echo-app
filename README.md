# echo-app

![Build and Push Docker Image](https://github.com/philipschmid/echo-app/actions/workflows/build.yaml/badge.svg) ![Go Syntax and Format Check](https://github.com/philipschmid/echo-app/actions/workflows/test.yaml/badge.svg)

This is a simple Go application that responds with a JSON payload containing various details. The JSON response includes:

- Timestamp
- Source IP
- Hostname
- Listener name
- HTTP version, HTTP method, and HTTP endpoint (HTTP and TLS listener only)
- gRPC method (gRPC listener only)
- Optionally, a customizable message, the (Kubernetes) node name, the HTTP request headers.

The application supports multiple listeners and functionalities:

- **HTTP Listener**: Responds with the JSON payload over HTTP.
- **TLS (HTTPS) Listener**:
  - Generates an in-memory self-signed TLS certificate.
  - Allows secure communication over a dedicated HTTPS port.
  - Returns the same JSON message over a TLS-encrypted HTTP connection.
- **TCP Listener**: Serves the same JSON message over a TCP connection (minus the request headers).
- **gRPC Listener**: Provides the same information using gRPC (minus the request headers).

These features make the application versatile for different types of network communication.

## Configuration Options
- `MESSAGE`: A customizable message to be returned in the JSON response. If not set, no message will be displayed.
- `NODE`: The name of the node where the app is running. This is typically used in a Kubernetes environment.
- `PORT`: The port number on which the server listens. Default is `8080`.
- `PRINT_HTTP_REQUEST_HEADERS`: Set to `true` to include HTTP request headers in the JSON response. By default, headers are not included.
- `TLS`: Set to `true` to enable TLS (HTTPS) support. By default, TLS is disabled.
- `TLS_PORT`: The port number on which the TLS server listens. Default is `8443`.
- `TCP`: Set to `true` to enable the TCP listener. By default, TCP is disabled.
- `TCP_PORT`: The port number on which the TCP server listens. Default is `9090`.
- `GRPC`: Set to `true` to enable the gRPC listener. By default, gRPC is disabled.
- `GRPC_PORT`: The port number on which the gRPC server listens. Default is `50051`.
- `LOG_LEVEL`: Set the logging level (`debug`, `info`, `warn`, `error`). Default is `info`.

## Makefile Targets
- `make`: Show the help message.
- `make build`: Build the Go application.
- `make vet`: Run `go vet` to examine Go source code and report suspicious constructs.
- `make lint`: Run `golangci-lint` to perform static code analysis.
- `make test`: Run the tests using `go test`.
- `make docker`: Build the Docker image using `docker buildx build`.
- `make run`: Build and run the Go application.

## Running the Application
To run the application, you can use the `make run` command:

```bash
make run
```

This will build the Go application (if not already built) and then execute it.

## Building the Docker Image
To build the Docker image, you can use the `make docker` command:

```bash
make docker
```

This will build a multi-arch Docker image for both `amd64` and `arm64` platforms.

## Standalone Container
Shell 1 (server):
```bash
docker run -it -p 8080:8080 ghcr.io/philipschmid/echo-app:main
# Optionally with a customized message:
docker run -it -p 8080:8080 -e MESSAGE="demo-env" ghcr.io/philipschmid/echo-app:main
# Optionally with a node name:
docker run -it -p 8080:8080 -e NODE="k8s-node-1" ghcr.io/philipschmid/echo-app:main
# Optionally include HTTP request headers in the response:
docker run -it -p 8080:8080 -e PRINT_HTTP_REQUEST_HEADERS="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable TLS:
docker run -it -p 8080:8080 -p 8443:8443 -e TLS="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable TCP:
docker run -it -p 8080:8080 -p 9090:9090 -e TCP="true" ghcr.io/philipschmid/echo-app:main
# Optionally enable gRPC:
docker run -it -p 8080:8080 -p 50051:50051 -e GRPC="true" ghcr.io/philipschmid/echo-app:main
```

Shell 2 (client):
```bash
curl http://localhost:8080/
```

You should see a similar client output like this:
```json
{
    "timestamp": "2024-05-28T19:50:10.289Z",
    "hostname": "83ff0b127ed6",
    "source_ip": "192.168.65.1",
    "listener": "HTTP"
}
{
    "timestamp": "2024-05-28T19:50:35.022Z",
    "message": "Hello World!",
    "hostname": "4495529ebd32",
    "source_ip": "192.168.65.1",
    "node": "k8s-node-1",
    "listener": "HTTP"
}
```

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:
```json
{
    "timestamp": "2024-05-28T20:21:23.363Z",
    "hostname": "3f96391b04f2",
    "source_ip": "192.168.65.1",
    "node": "k8s-node-1",
    "listener": "HTTP",
    "headers": {
        "Accept": [
            "*/*"
        ],
        "User-Agent": [
            "curl/8.6.0"
        ]
    }
}
```

If `TLS` is enabled, you can test the HTTPS listener:
```bash
curl -k https://localhost:8443/
```

To test the TCP listener using netcat:
```bash
nc localhost 9090
```

You should see a similar output like this:
```json
{
    "timestamp": "2024-05-28T19:50:10.289Z",
    "hostname": "83ff0b127ed6",
    "source_ip": "127.0.0.1",
    "listener": "TCP"
}
{
    "timestamp": "2024-05-28T19:50:35.022Z",
    "message": "Hello World!",
    "hostname": "4495529ebd32",
    "source_ip": "127.0.0.1",
    "node": "k8s-node-1",
    "listener": "TCP"
}
```

To test the gRPC listener, you can use a gRPC client like `grpcurl`:

```bash
grpcurl -plaintext localhost:50051 echo.EchoService/Echo
```

You should see a similar output like this:
```json
{
  "timestamp": "2024-08-01T13:55:45.228Z",
  "sourceIp": "192.168.65.1",
  "hostname": "efa892e16a74",
  "listener": "gRPC"
}
{
  "timestamp": "2024-08-01T13:55:45.228Z",
  "message": "Hello World!",
  "sourceIp": "192.168.65.1",
  "hostname": "a96e5c48f68c",
  "listener": "gRPC",
  "node": "k8s-node-1",
  "grpc_method": "/echo.EchoService/Echo"
}
```

## Kubernetes
Apply the following manifests to deploy the echo-app with the `NODE` environment variable set to the name of the Kubernetes node using the Downward API:
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-app-config
data:
  MESSAGE: "demo-env"
  # Add the PRINT_HTTP_REQUEST_HEADERS key with a value of "true" to include headers in the response
  PRINT_HTTP_REQUEST_HEADERS: "true"
  # Add the TLS key with a value of "true" to enable TLS
  TLS: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-app-deployment
  labels:
    app: echo-app
spec:
  replicas: 2  # Adjust the number of replicas as needed
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
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
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
        - containerPort: 8080
        - containerPort: 8443
        - containerPort: 9090
        - containerPort: 50051
        env:
        - name: MESSAGE
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: MESSAGE
        #  Add the PRINT_HTTP_REQUEST_HEADERS environment variable
        - name: PRINT_HTTP_REQUEST_HEADERS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: PRINT_HTTP_REQUEST_HEADERS
        # Add the TLS environment variable
        - name: TLS
          valueFrom:
            configMapKeyRef:
              name: echo-app-config
              key: TLS
        # Add the NODE environment variable using the downward API
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
  - name: https
    protocol: TCP
    port: 8443
    targetPort: 8443
  - name: tcp
    protocol: TCP
    port: 9090
    targetPort: 9090
  - name: grpc
    protocol: TCP
    port: 50051
    targetPort: 50051
  type: ClusterIP
```

Shell 2 (client):
```bash
kubectl run netshoot --rm -it --image=nicolaka/netshoot -- curl http://echo-app-service:8080
```

You should see a similar client output like this:
```json
{"timestamp":"2024-05-28T19:50:35.022Z","message":"demo-env","hostname":"echo-app-deployment-5d8f8b8b8b-9t4kq","source_ip":"10.1.0.1","node":"k8s-node-1","listener":"HTTP"}
```

If `PRINT_HTTP_REQUEST_HEADERS` is set to `true`, the response will also include the request headers:
```json
{"timestamp":"2024-05-28T20:21:23.363Z","message":"demo-env","hostname":"echo-app-deployment-5d8f8b8b8b-9t4kq","source_ip":"10.1.0.1","node":"k8s-node-1","listener":"HTTP","headers":{"Accept":["*/*"],"User-Agent":["curl/8.6.0"]}}
```

### Ingress Example
For example, if you're running a cluster with Cilium installed like this: https://gist.github.com/PhilipSchmid/bf4e4d2382678959f29f6e0d7b9b4725
```yaml
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echo
  annotations:
    cert-manager.io/cluster-issuer: "lets-encrypt-prod"
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
  tls:
  - hosts:
    - echo.<ip-of-ingress-lb-service>.sslip.io
    secretName: echo-app-tls
```

### Gateway API Example
For example, if you're running a cluster with Cilium installed like this: https://gist.github.com/PhilipSchmid/bf4e4d2382678959f29f6e0d7b9b4725
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
  name: tcp-echo-gw
  namespace: infra
spec:
  gatewayClassName: cilium
  listeners:
  - name: tcp
    protocol: TCP
    port: 9090
    allowedRoutes:
      namespaces:
        from: All
      kinds:
      - kind: TCPRoute
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
    protocol: GRPC
    port: 50051
    allowedRoutes:
      namespaces:
        from: All
      kinds:
      - kind: GRPCRoute
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
kind: TCPRoute
metadata:
  name: tcp-echo
spec:
  parentRefs:
  - name: tcp-echo-gw
    namespace: infra
    sectionName: tcp
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 9090
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GRPCRoute
metadata:
  name: grpc-echo
spec:
  parentRefs:
  - name: grpc-echo-gw
    namespace: infra
  rules:
  - backendRefs:
    - name: echo-app-service
      port: 50051
```

Testing `HTTPRoute`:
```bash
$ while true; do curl -sSL http://echo.<ip-of-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
{
  "timestamp": "2024-07-31T09:04:14.801Z",
  "message": "demo-env",
  "source_ip": "10.0.0.169",
  "hostname": "echo-app-deployment-85f85574bb-cbv9p",
  "node": "aks-nodepool1-15164467-vmss000000",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.6.0"
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
      "6821c0bc-361c-4f15-837a-be3dc025ff78"
    ]
  }
}
```

Testing `TLSRoute`:
```bash
$ while true; do curl -sSLk https://tls-echo.<ip-of-tls-echo-gw-lb-service>.sslip.io | jq; sleep 2; done
{
  "timestamp": "2024-07-31T09:06:43.293Z",
  "message": "demo-env",
  "source_ip": "10.0.2.214",
  "hostname": "echo-app-deployment-85f85574bb-rspj9",
  "node": "aks-nodepool1-15164467-vmss000002",
  "headers": {
    "Accept": [
      "*/*"
    ],
    "User-Agent": [
      "curl/8.6.0"
    ]
  }
}
```

Testing `TCPRoute`:
```bash
$ while true; do nc <ip-of-tcp-echo-gw-lb-service>.sslip.io 9090; sleep 2; done
{
  "timestamp": "2024-07-31T09:08:43.293Z",
  "message": "demo-env",
  "source_ip": "10.0.2.214",
  "hostname": "echo-app-deployment-85f85574bb-rspj9",
  "node": "aks-nodepool1-15164467-vmss000002",
  "listener": "TCP"
}
```

Testing `GRPCRoute`:
```bash
$ while true; do grpcurl -plaintext <ip-of-grpc-echo-gw-lb-service>.sslip.io:50051 echo.EchoService/Echo; sleep 2; done
{
  "timestamp": "2024-07-31T09:10:43.293Z",
  "message": "demo-env",
  "source_ip": "10.0.2.214",
  "hostname": "echo-app-deployment-85f85574bb-rspj9",
  "node": "aks-nodepool1-15164467-vmss000002",
  "listener": "gRPC",
  "grpc_method": "/echo.EchoService/Echo"
}
```

## Credit
Basic idea (& source code) is taken from https://cloud.google.com/kubernetes-engine/docs/samples/container-hello-app.