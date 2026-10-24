# Usage

[README](../README.md) · [Configuration](configuration.md) · [Kubernetes](kubernetes.md)

## Connect to the listeners

HTTP on port 8080 and the admin listener on port 3000 start by default. HTTP
supports HTTP/1.1 and, with `--h2c`, cleartext HTTP/2.0. HTTPS supports HTTP/1.1
and HTTP/2.0 when enabled with `--tls`. Enable the other listeners when needed:

```bash
./echo-app --h2c --tls --tcp --grpc --quic --print-http-request-headers
```

Or run all listeners with Docker, publishing the ports only on localhost:

```bash
docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:8443:8443 \
  -p 127.0.0.1:9090:9090 -p 127.0.0.1:50051:50051 \
  -p 127.0.0.1:4433:4433/udp -p 127.0.0.1:3000:3000 \
  ghcr.io/philipschmid/echo-app:main --h2c --tls --tcp --grpc --quic
```

| Connection | Default address | Client example |
| --- | --- | --- |
| HTTP/1.1 | `http://localhost:8080/` | `curl -sS http://localhost:8080/` |
| HTTP/2.0 cleartext (h2c) | `http://localhost:8080/` with `--h2c` | `curl --http2-prior-knowledge http://localhost:8080/` |
| HTTPS, HTTP/1.1 or HTTP/2.0 | `https://localhost:8443/` with `--tls` | `curl -sk --http2 https://localhost:8443/` |
| HTTP/3 over QUIC | `https://localhost:4433/` with `--quic` | `curl -sk --http3-only https://localhost:4433/` |
| Raw TCP | `localhost:9090` with `--tcp` | `nc localhost 9090` |
| gRPC | `localhost:50051` with `--grpc` | `grpcurl -plaintext -d '{}' localhost:50051 echo.EchoService/Echo` |
| WebSocket | `ws://localhost:8080/ws` | Use an HTTP/1.1 WebSocket client. |
| Secure WebSocket | `wss://localhost:8443/ws` with `--tls` | Use a WebSocket client that trusts the server certificate. |

The curl HTTP/2 and HTTP/3 examples require a build with the corresponding
protocol support. `-k` skips certificate verification for the generated demo
certificate; use a trusted certificate for verified connections.

HTTP returns metadata at arbitrary paths except `/ws`. TCP sends one JSON
metadata document and closes the
connection. HTTP request bodies are consumed within the configured limit and
discarded; gRPC accepts an empty request.
These metadata responses do not echo application payloads.

## Read request diagnostics

A plain HTTP response looks like this:

```json
{
  "timestamp": "2026-10-24T12:00:00Z",
  "hostname": "demo-host",
  "listener": "HTTP",
  "source_ip": "127.0.0.1",
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/example",
  "http_host": "localhost:8080",
  "http_request_uri": "/example?item=42",
  "remote_addr": "127.0.0.1:54000",
  "local_addr": "127.0.0.1:8080"
}
```

`--message` and `--node` add environment or Kubernetes identity. HTTPS and
HTTP/3 responses also report TLS version, cipher suite, SNI, and the negotiated
application protocol. The original request URI preserves escaping and query
parameters; `http_endpoint` contains the decoded path.

`source_ip` always identifies the actual socket peer. Forwarded headers appear
in the optional header output and never replace the peer address.

Enable header output with `--print-http-request-headers`. Authorization and
cookie headers, and fields whose names contain secret, token, or key, are
redacted by default. `--unsafe-headers` disables this redaction in responses and
debug metadata logs; use it only with synthetic credentials. HTTP diagnostic
responses use `Cache-Control: no-store`.

## Control HTTP responses

Stop any running instance, then enable the controls in one terminal:

```bash
./echo-app --response-controls
```

In another terminal:

```bash
curl 'http://localhost:8080/example?status=503&delay=200ms'
curl 'http://localhost:8080/example?size=1024'
```

| Query parameter | Accepted values | Effect |
| --- | --- | --- |
| `status` | 200–599 | Set the response status; default is 200. |
| `delay` | Go duration, 0–10 seconds | Delay the response; cancellation stops the wait. |
| `size` | 0–1,048,576 bytes | Return exactly that many zero bytes as `application/octet-stream`. |

Without `size`, the body remains JSON. HEAD and bodyless statuses retain their
normal HTTP semantics. Invalid, repeated, or out-of-range control parameters
return `400`. Without `--response-controls`, these parameters have no special
effect.

## Use certificates and mutual TLS

Self-signed certificates are the default for demos. Supply both a PEM
certificate chain and its private key to use a real trust chain. Start a new
instance in one terminal:

```bash
./echo-app --tls --quic --grpc --grpc-tls \
  --tls-cert-file=server-chain.pem --tls-key-file=server-key.pem
```

Connect from another terminal; the server certificate must cover `localhost`:

```bash
curl --cacert=server-ca.pem https://localhost:8443/
```

Add `--tls-client-ca-file=client-ca.pem` to the server command and restart it
to require and verify client certificates against that CA bundle. Then connect
with a client certificate:

```bash
curl --cacert=server-ca.pem --cert=client.pem --key=client-key.pem \
  https://localhost:8443/

grpcurl -cacert server-ca.pem -cert client.pem -key client-key.pem \
  -d '{}' localhost:50051 echo.EchoService/Echo
```

These settings apply to HTTPS and HTTP/3, and to gRPC only with `--grpc-tls`.
Plain HTTP, TCP, default plaintext gRPC, and the admin listener remain
unencrypted; restrict those ports separately. Certificate changes take effect
after restarting the app.

## WebSocket behavior

`/ws` echoes complete text and binary messages, including fragmented messages.
Each message is limited to the smaller of `--max-request-size` and 1 MiB. Reads
and writes have 30-second deadlines. Invalid UTF-8 text closes with status 1007;
binary messages may contain arbitrary bytes.

Browser Origin headers must use HTTP or HTTPS and match the request host and
port. Clients without an Origin header are accepted. An HTTPS origin can reach
a plain HTTP backend behind TLS-terminating ingress. Connections close during
server shutdown.

The endpoint requires an HTTP/1.1 upgrade without an HTTP request body. HTTP/2
and HTTP/3 WebSocket upgrades return `426`.

## Health, readiness, and metrics

Use the dedicated admin listener for Kubernetes probes and Prometheus scraping:

```bash
curl -fsS http://localhost:3000/health   # healthy
curl -fsS http://localhost:3000/ready    # ready
curl -fsS http://localhost:3000/metrics
```

`/health` reports process liveness. `/ready` requires every configured listener
to bind and any configured external readiness probe to pass; otherwise it
returns `503`. Probe results are cached in the background, so `/ready` does not
make outbound calls. A listener failure terminates the process. External
dependency failures affect readiness, not liveness.

The gRPC listener exposes reflection and the standard health service:

```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -d '{"service":"echo.EchoService"}' \
  localhost:50051 grpc.health.v1.Health/Check
grpcurl -plaintext -d '{"service":"echo.EchoService"}' \
  localhost:50051 grpc.health.v1.Health/Watch
```

`Check` and `Watch` accept an empty service name or `echo.EchoService` and follow
the same readiness state. Health watches finish during shutdown.

Prometheus metrics include:

```text
echo_app_requests_total{listener="HTTP",method="GET",endpoint="/"}
echo_app_request_duration_seconds_bucket{listener="HTTP",method="GET",endpoint="/",le="0.1"}
echo_app_errors_total{listener="HTTP",error_type="write_error"}
echo_app_active_connections{listener="TCP"}
```

Keep the admin listener private and restrict diagnostic traffic with network
policies. Pin an image version or digest for repeatable deployments.

## Check an external readiness dependency

Echo-app makes outbound HTTP, TCP, or ICMP checks against a dependency; the
cached result contributes to `/ready`.

External checks are disabled by default. Configure a probe type and target
together; incomplete or invalid settings fail at startup. See
[configuration](configuration.md#external-readiness) for intervals, timeouts,
and expected HTTP status.

```bash
# Expect HTTP 200 within two seconds.
ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=http \
ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=https://upstream.example.com/ready \
ECHO_APP_EXTERNAL_READINESS_PROBE_TIMEOUT=2s \
ECHO_APP_EXTERNAL_READINESS_PROBE_INTERVAL=10s \
./echo-app

# Require a TCP connection to the dependency.
ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=tcp \
ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=db.example.com:5432 \
./echo-app
```

ICMP runs in-process without a `ping` binary. DNS resolution uses the probe
deadline and preserves IPv4 preference for dual-stack hostnames; an IPv6 literal
selects IPv6 directly.

### ICMP privileges

The image runs as user 65532 without capabilities for ordinary operation. Raw
ICMP is an explicit exception: on the tested Docker runtime, adding `NET_RAW`
alone does not give that non-root user access to raw sockets. This mode was
verified with a UID 0 override, a read-only filesystem, and only `NET_RAW`:

```bash
docker run --rm --user 0:0 --read-only \
  --cap-drop=ALL --cap-add=NET_RAW --security-opt=no-new-privileges \
  -p 127.0.0.1:3000:3000 \
  -e ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE=icmp \
  -e ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET=127.0.0.1 \
  ghcr.io/philipschmid/echo-app:main
```

Use HTTP or TCP checks to retain the non-root defaults. The
[Kubernetes example](kubernetes.md) drops all capabilities; raw ICMP needs an
explicitly adjusted security context.
