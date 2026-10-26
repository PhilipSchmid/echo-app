# Kubernetes

[README](../README.md) · [Configuration](configuration.md) · [Usage](usage.md)

This example enables all echo listeners through a ClusterIP Service on
80/TCP, 443/TCP+UDP, 50051/TCP and 9090/TCP, matching Compose. Admin probes
use a separate ClusterIP Service. The pod runs as UID 65532 with a read-only
filesystem, no privilege escalation and all capabilities dropped.

Copy the manifest into `echo-observatory.yaml`, adjust the image and configuration, then
apply it to your chosen namespace:

```bash
kubectl apply -f echo-observatory.yaml
```

The example pins version 2.1.0. Use a release tag or image digest
for repeatable deployments.

## Deployment and Service

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: echo-observatory-config
data:
  ECHO_OBSERVATORY_MESSAGE: "demo-env"
  ECHO_OBSERVATORY_PRINT_HTTP_REQUEST_HEADERS: "true"
  ECHO_OBSERVATORY_PROTECTION: "false"
  ECHO_OBSERVATORY_H2C: "true"
  ECHO_OBSERVATORY_TLS: "true"
  ECHO_OBSERVATORY_HTTP_ADVERTISE_PORT: "80"
  ECHO_OBSERVATORY_TLS_ADVERTISE_PORT: "443"
  ECHO_OBSERVATORY_TCP_ADVERTISE_PORT: "9090"
  ECHO_OBSERVATORY_GRPC_ADVERTISE_PORT: "50051"
  ECHO_OBSERVATORY_QUIC_ADVERTISE_PORT: "443"
  ECHO_OBSERVATORY_GRPC_TLS: "true"
  ECHO_OBSERVATORY_QUIC: "true"
  ECHO_OBSERVATORY_GRPC: "true"
  ECHO_OBSERVATORY_TCP: "true"
  ECHO_OBSERVATORY_METRICS: "true"
  # Optional: mark this pod not-ready when a required upstream stops responding.
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE: "http"
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET: "https://upstream.example.com/ready"
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_METHOD: "GET"
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS: "200"
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TIMEOUT: "2s"
  # ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_INTERVAL: "10s"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo-observatory-deployment
  labels:
    app: echo-observatory
spec:
  replicas: 3
  selector:
    matchLabels:
      app: echo-observatory
  template:
    metadata:
      labels:
        app: echo-observatory
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: echo-observatory
        image: ghcr.io/philipschmid/echo-observatory:2.1.0
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
            name: echo-observatory-config
        env:
        - name: ECHO_OBSERVATORY_NODE
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
  name: echo-observatory-service
spec:
  selector:
    app: echo-observatory
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: tls
    port: 443
    targetPort: 8443
  - name: quic
    port: 443
    targetPort: 4433
    protocol: UDP
  - name: tcp
    port: 9090
    targetPort: 9090
  - name: grpc
    port: 50051
    targetPort: 50051
  ipFamilyPolicy: PreferDualStack
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: echo-observatory-admin
spec:
  selector:
    app: echo-observatory
  ports:
  - name: metrics
    port: 3000
    targetPort: 3000
  ipFamilyPolicy: PreferDualStack
  type: ClusterIP
```

`/health` and `/ready` are on the `metrics` port, not the echo traffic ports.
Liveness remains independent of external services. If you enable external
readiness, replace the example target with a real dependency; the pod remains
not ready until that probe succeeds.

The traffic Service exposes `/ui/` and JSON paths over HTTP, HTTPS and native
HTTP/3. Its HTTPS responses advertise UDP 443 for browser discovery. Publish
the traffic Service through a TCP/UDP-capable LoadBalancer or gateway with
these same exposed ports; leave `echo-observatory-admin` as ClusterIP. Your
controller must support mixed TCP/UDP Services on the same address.
Both Services use `PreferDualStack`: dual-stack clusters can assign IPv4 and
IPv6 service addresses, while single-stack clusters retain their available
family. Pod addressing, CNI routing and the external load balancer must also
support IPv6 for end-to-end IPv6 traffic.
Mount a trusted certificate for your hostname before using it publicly.
If UDP is not exposed, remove `ECHO_OBSERVATORY_QUIC_ADVERTISE_PORT`. Restrict
admin access with your cluster network policies; ClusterIP alone does not
isolate it from other pods.

## Connect through a Service or ingress

| Traffic | Backend | Routing requirement |
| --- | --- | --- |
| HTTP | TCP 8080 | HTTP/1.1; enable `--h2c` for cleartext HTTP/2.0. Expose on TCP 80; route JSON and `/ui/` paths to this listener. |
| HTTPS | TCP 8443 for backend TLS | HTTP/1.1 and HTTP/2.0 with `--tls`. Expose on TCP 443. Configure upstream TLS, or terminate TLS at ingress and forward HTTP to port 8080. |
| WebSocket | `/ws` on TCP 8080 or 8443 | Preserve the HTTP/1.1 upgrade; HTTP/2 and HTTP/3 upgrades are unsupported. |
| gRPC | TCP 50051 | Expose TCP 50051 with an HTTP/2-capable backend; this profile enables `--grpc-tls`. |
| HTTP/3 | UDP 4433 | Expose as UDP 443, including `/ui/`. Forward UDP through a compatible Service or gateway; an HTTP route alone does not expose this listener. |
| Raw TCP | TCP 9090 | Use a TCP-capable Service or gateway rather than HTTP path routing. |
| Admin | TCP 3000 | Keep `/metrics`, `/health`, and `/ready` private; do not add them to the public traffic ingress. |

Controller configuration depends on where TLS terminates and which transports
it supports. A client speaking HTTP/3 to an ingress may still reach echo-observatory
through HTTP/1.1 or HTTP/2; the response reports the protocol received by the
app. Forwarded HTTP headers never replace `source_ip`, which identifies the
socket peer by default. A TCP gateway that sends PROXY protocol can supply
original client metadata with the application's opt-in
[trusted PROXY protocol mode](configuration.md#trusted-proxy-protocol); the
provided manifests leave it disabled. With TLS termination at ingress, backend mutual TLS authenticates the
ingress connection rather than the original client.

## Public access

Serve the echo endpoints and `/ui/` subtree through a trusted HTTPS ingress,
preserving WebSocket upgrades. If the ingress strips a path prefix, route the
request view, workbench, examples, connection guide, and echo requests through
that same prefix.
The UI's public HTTP examples use the URL visitors actually reached.

Keep the admin listener private. Enable shared rate and transfer budgets with
`ECHO_OBSERVATORY_PROTECTION=true` and choose limits for the deployment;
each replica has its own allowance.
Use durable quota state when restarts must preserve it, and retain network-level
protection for traffic that never reaches the app. Publish TCP,
gRPC, or UDP endpoints separately when needed; configured backend ports alone
do not establish public reachability.

Use synthetic data and retain credential redaction. Configure log access and
retention for your installation; see [privacy and logging](usage.md#privacy-and-logging).

## Optional settings

- Mount PEM certificate and key files and set the [TLS options](configuration.md#tls-and-client-authentication) for verified backend TLS or mutual TLS. Plain HTTP, TCP, and admin traffic are unaffected by those settings.
- Enable `ECHO_OBSERVATORY_RESPONSE_CONTROLS` only when you need bounded status, delay, or response-size simulation.
- Set `ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE` and its target together to test a required dependency. Avoid making liveness depend on that dependency.
- Raw ICMP requires an explicitly adjusted security context. The baseline manifest deliberately prevents raw sockets; prefer HTTP or TCP probes to keep its non-root settings. See the [verified Docker exception](usage.md#check-an-external-readiness-dependency).
