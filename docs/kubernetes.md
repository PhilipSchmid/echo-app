# Kubernetes

[README](../README.md) · [Configuration](configuration.md) · [Usage](usage.md)

This example enables all echo listeners and exposes them through a ClusterIP
Service. Probes use the dedicated admin listener. The pod runs as UID 65532 with
a read-only filesystem, no privilege escalation, and all capabilities dropped.
External readiness checks are optional and commented out.

Copy the manifest into `echo-app.yaml`, adjust the image and configuration, then
apply it to your chosen namespace:

```bash
kubectl apply -f echo-app.yaml
```

The `main` image tag is convenient for demos. Pin a release version or image
digest for repeatable deployments.

## Deployment and Service

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

`/health` and `/ready` are on the `metrics` port, not the echo traffic ports.
Liveness remains independent of external services. If you enable external
readiness, replace the example target with a real dependency; the pod remains
not ready until that probe succeeds.

## Connect through a Service or ingress

| Traffic | Backend | Routing requirement |
| --- | --- | --- |
| HTTP | TCP 8080 | HTTP/1.1; enable `--h2c` for cleartext HTTP/2.0. Route metadata paths to this listener. |
| HTTPS | TCP 8443 for backend TLS | HTTP/1.1 and HTTP/2.0 with `--tls`. Configure upstream TLS, or terminate TLS at ingress and forward HTTP to port 8080. |
| WebSocket | `/ws` on TCP 8080 or 8443 | Preserve the HTTP/1.1 upgrade; HTTP/2 and HTTP/3 upgrades are unsupported. |
| gRPC | TCP 50051 | Use a gRPC-capable HTTP/2 backend; enable `--grpc-tls` for encrypted backend connections. |
| HTTP/3 | UDP 4433 | Expose UDP through a compatible Service or gateway; an HTTP route alone does not expose this listener. |
| Raw TCP | TCP 9090 | Use a TCP-capable Service or gateway rather than HTTP path routing. |
| Admin | TCP 3000 | Keep `/metrics`, `/health`, and `/ready` private; do not add them to the public traffic ingress. |

Controller configuration depends on where TLS terminates and which transports
it supports. A client speaking HTTP/3 to an ingress may still reach echo-app
through HTTP/1.1 or HTTP/2; the response reports the protocol received by the
app. Forwarded headers never replace `source_ip`, which identifies the socket
peer. With TLS termination at ingress, backend mutual TLS authenticates the
ingress connection rather than the original client.

## Public demo

Serve the echo endpoints through a trusted HTTPS ingress, preserving
WebSocket upgrades. Publish only the protocol listeners needed by visitors.

Keep the admin listener private. Apply ingress traffic limits appropriate for
the public demo; the app's connection, body-size, and response-control limits do
not replace a deployment's request-rate and bandwidth budgets. Publish TCP,
gRPC, or UDP endpoints separately when needed; configured backend ports alone
do not establish public reachability.

Use synthetic data and retain credential redaction. Review server/proxy logging
and expose only the deployment metadata you intend visitors to inspect.

## Optional settings

- Mount PEM certificate and key files and set the [TLS options](configuration.md#tls-and-client-authentication) for verified backend TLS or mutual TLS. Plain HTTP, TCP, and admin traffic are unaffected by those settings.
- Enable `ECHO_APP_RESPONSE_CONTROLS` only when you need bounded status, delay, or response-size simulation.
- Set `ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE` and its target together to test a required dependency. Avoid making liveness depend on that dependency.
- Raw ICMP requires an explicitly adjusted security context. The baseline manifest deliberately prevents raw sockets; prefer HTTP or TCP probes to keep its non-root settings. See the [verified Docker exception](usage.md#check-an-external-readiness-dependency).
