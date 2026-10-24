# Configuration

[Back to README](../README.md) · [Usage examples](usage.md)

Set environment variables or command-line flags. Explicit flags override the
matching environment variables. Run `./echo-app --help` for the flag list.

## Identity and diagnostics

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_APP_MESSAGE` | `--message` | Empty | Custom response message; maximum 1,024 bytes. |
| `ECHO_APP_NODE` | `--node` | Empty | Node identifier included in responses. |
| `ECHO_APP_LOG_LEVEL` | `--log-level` | `info` | Logging level: `trace`, `debug`, `info`, `warn` (`warning`), `error`, `fatal`, or `panic`. |
| `ECHO_APP_PRINT_HTTP_REQUEST_HEADERS` | `--print-http-request-headers` | `false` | Include request headers in HTTP responses and diagnostic logs. |
| `ECHO_APP_UNSAFE_HEADERS` | `--unsafe-headers` | `false` | Reveal sensitive header and debug metadata values. Use synthetic credentials only. |
| `ECHO_APP_MAX_REQUEST_SIZE` | `--max-request-size` | `10485760` | HTTP request body limit in bytes: 10 MiB. Oversized metadata requests return `413`; bodies are discarded after reading. |
| `ECHO_APP_RESPONSE_CONTROLS` | `--response-controls` | `false` | Enable the bounded `status`, `delay`, and `size` query controls described in [Usage](usage.md). |

Sensitive headers are redacted by default. `--unsafe-headers` does not enable HTTP
header output by itself; also set `--print-http-request-headers` when needed.

## Listeners

HTTP is always enabled. All listeners bind to all interfaces on their configured
port. Publish or expose only the ports needed for a test.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_APP_HTTP_PORT` | `--http-port` | `8080` | HTTP and WebSocket port, over TCP. |
| `ECHO_APP_H2C` | `--h2c` | `false` | Accept cleartext HTTP/2.0 (h2c) alongside HTTP/1.1 on the HTTP port. |
| `ECHO_APP_TLS` | `--tls` | `false` | Enable HTTPS with HTTP/1.1 and HTTP/2.0, plus secure WebSocket connections. |
| `ECHO_APP_TLS_PORT` | `--tls-port` | `8443` | HTTPS port, over TCP. |
| `ECHO_APP_TCP` | `--tcp` | `false` | Enable the raw TCP metadata responder. |
| `ECHO_APP_TCP_PORT` | `--tcp-port` | `9090` | Raw TCP port. |
| `ECHO_APP_GRPC` | `--grpc` | `false` | Enable gRPC echo, reflection, and standard health services. |
| `ECHO_APP_GRPC_PORT` | `--grpc-port` | `50051` | gRPC port, over TCP. |
| `ECHO_APP_GRPC_TLS` | `--grpc-tls` | `false` | Use TLS on the gRPC port; requires `--grpc`. |
| `ECHO_APP_QUIC` | `--quic` | `false` | Enable HTTP/3 over QUIC. |
| `ECHO_APP_QUIC_PORT` | `--quic-port` | `4433` | HTTP/3 port, over UDP. |
| `ECHO_APP_METRICS` | `--metrics` | `true` | Enable the admin listener with `/metrics`, `/health`, and `/ready`. |
| `ECHO_APP_METRICS_PORT` | `--metrics-port` | `3000` | Admin HTTP port, over TCP. |

Use `--metrics=false` or `ECHO_APP_METRICS=false` to disable the admin listener.
WebSocket upgrades use HTTP/1.1 at `/ws`; HTTP/2 and HTTP/3 upgrades return `426`.

## TLS and client authentication

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_APP_TLS_CERT_FILE` | `--tls-cert-file` | Empty | PEM server certificate chain. Supply with the private key. |
| `ECHO_APP_TLS_KEY_FILE` | `--tls-key-file` | Empty | PEM private key matching the server certificate. |
| `ECHO_APP_TLS_CLIENT_CA_FILE` | `--tls-client-ca-file` | Empty | PEM client CA bundle; require and verify client certificates. |

Without custom files, encrypted listeners use a cached, in-memory self-signed
certificate for demos. Custom certificate and key files must be supplied
together, and TLS files require an enabled HTTPS, HTTP/3, or TLS-enabled gRPC
listener. Restart the application after changing certificate files.

Client authentication applies to encrypted listeners only. Plain HTTP, TCP,
default plaintext gRPC, and the admin listener remain unencrypted. See
[Usage](usage.md) for trust-chain and mutual TLS examples.

## External readiness

An optional background probe contributes to readiness. Requests to `/ready` read
its cached result without making outbound calls.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_APP_EXTERNAL_READINESS_PROBE_TYPE` | `--external-readiness-probe-type` | `none` | Probe type: `none`, `http`, `tcp`, or `icmp`. Compatibility aliases: `https` and `ping`. |
| `ECHO_APP_EXTERNAL_READINESS_PROBE_TARGET` | `--external-readiness-probe-target` | Empty | HTTP(S) URL, TCP `host:port`, or ICMP hostname/IP. |
| `ECHO_APP_EXTERNAL_READINESS_PROBE_INTERVAL` | `--external-readiness-probe-interval` | `10s` | Time between background checks; must be positive. |
| `ECHO_APP_EXTERNAL_READINESS_PROBE_TIMEOUT` | `--external-readiness-probe-timeout` | `2s` | Per-check deadline, including ICMP DNS resolution; must be positive. |
| `ECHO_APP_EXTERNAL_READINESS_HTTP_METHOD` | `--external-readiness-http-method` | `GET` | Method used for HTTP readiness checks. |
| `ECHO_APP_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS` | `--external-readiness-http-expected-status` | `200` | Expected HTTP status; accepted configuration range is 100–599. |

A probe type requires a target, and a target requires a probe type. HTTP targets
must use HTTP or HTTPS; TCP targets need a hostname and a valid numeric port.
Raw ICMP needs the explicit runtime privileges described in [Usage](usage.md).

## Startup validation

Enabled ports must be in the range 1–65535. TCP listeners cannot share a port;
HTTP/3 may use the same number because it uses UDP. The request-size limit must
be positive. Invalid settings stop startup, and a listener failure terminates
the process.

Readiness requires every enabled listener to bind and any external probe to
pass. Liveness does not depend on an external service. On shutdown, readiness
becomes false before listeners drain.
