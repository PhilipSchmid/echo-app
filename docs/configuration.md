# Configuration

[Back to README](../README.md) · [Usage examples](usage.md) · [Privacy and logging](usage.md#privacy-and-logging)

Set environment variables or command-line flags. Explicit flags override the
matching environment variables. Run `./echo-observatory --help` for the flag list.
Version 2.0 uses the `ECHO_OBSERVATORY_` prefix; see [upgrading from 1.x](migration-v2.md).

## Self-hosted defaults

With no options, the binary serves HTTP/WebSocket and the UI on port 8080,
plus admin health, readiness and metrics on port 3000. Keep the admin port private.
Additional protocols are opt-in, so a simple install opens no extra listeners.
The [Compose profile](usage.md#connect-to-the-listeners) enables all protocols
with the standard exposed ports and offline, self-signed certificates.

No rate or daily transfer quota applies unless `--protection` is enabled.
Request/response size limits, connection/concurrency limits and timeouts still
apply independently. Header output and response simulation remain opt-in;
credential redaction stays enabled. ACME, external probes, PROXY protocol,
search indexing and port advertisements require explicit configuration.
The defaults work offline without a domain or writable state directory.

## Identity and diagnostics

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_MESSAGE` | `--message` | Empty | Custom response message; maximum 1,024 bytes. |
| `ECHO_OBSERVATORY_NODE` | `--node` | Empty | Node identifier included in responses. |
| `ECHO_OBSERVATORY_LOG_LEVEL` | `--log-level` | `info` | Logging level: `trace`, `debug`, `info`, `warn` (`warning`), `error`, `fatal`, or `panic`. |
| `ECHO_OBSERVATORY_PRINT_HTTP_REQUEST_HEADERS` | `--print-http-request-headers` | `false` | Include request headers in HTTP responses and, at debug level, diagnostic logs. |
| `ECHO_OBSERVATORY_UNSAFE_HEADERS` | `--unsafe-headers` | `false` | Reveal sensitive headers in returned HTTP diagnostics only. Use synthetic credentials only. |
| `ECHO_OBSERVATORY_MAX_REQUEST_SIZE` | `--max-request-size` | `65536` | Incoming HTTP body / protocol message limit: 64 KiB. Oversized HTTP bodies return `413`; bodies are discarded after reading. |
| `ECHO_OBSERVATORY_MAX_RESPONSE_SIZE` | `--max-response-size` | `65536` | Echo response / outgoing message limit: 64 KiB. Maximum 1 MiB; `0` selects the legacy 1 MiB ceiling. UI HTML/assets are excluded; transfer budgets still apply. |
| `ECHO_OBSERVATORY_RESPONSE_CONTROLS` | `--response-controls` | `false` | Enable the bounded `status`, `delay`, and `size` query controls described in [Usage](usage.md). |

Returned diagnostics redact common credential headers by default using name
matching, including password, passphrase, and credential names. Server diagnostic
logs always apply this filter, even with `--unsafe-headers`. It is not an
exhaustive content filter; see [privacy and logging](usage.md#privacy-and-logging).
`--unsafe-headers` does not enable HTTP header output by itself; also set
`--print-http-request-headers` when needed.

## Abuse protection

Opt in with `--protection` when exposing an instance publicly or when you need
traffic budgets. One guard shares budgets across echo/UI listeners; the admin
listener is excluded. The rate, allowance and state-file options below apply
only when protection is enabled; setting one does not enable protection itself.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_PROTECTION` | `--protection` | `false` | Enable shared operation, connection and transfer budgets. |
| `ECHO_OBSERVATORY_REQUEST_RATE` | `--request-rate` | `20` | Sustained operations/second and, independently, new connections/second. |
| `ECHO_OBSERVATORY_PEER_REQUEST_RATE` | `--peer-request-rate` | `5` | Operations and connections/second per client address (socket peer by default, validated original address in PROXY mode); `0` disables peer limits. IPv6 clients share a `/64`. |
| `ECHO_OBSERVATORY_INGRESS_BYTES_PER_SECOND` | `--ingress-bytes-per-second` | `262144` | Sustained incoming public transport bytes/second: 256 KiB/s. |
| `ECHO_OBSERVATORY_EGRESS_BYTES_PER_SECOND` | `--egress-bytes-per-second` | `1048576` | Sustained outgoing public transport bytes/second: 1 MiB/s. |
| `ECHO_OBSERVATORY_DAILY_EGRESS_BYTES` | `--daily-egress-bytes` | `1073741824` | Shared outgoing transport allowance per UTC day: 1 GiB. |
| `ECHO_OBSERVATORY_TRANSFER_STATE_FILE` | `--transfer-state-file` | Empty | Optional private persistent daily-budget file; empty resets allowance on process restart. |

Choose budgets for your deployment. For public instances, use a private writable
`--transfer-state-file` when restarts must preserve the daily allowance. These
are application limits, not a provider billing cap; network overhead and traffic
that never reaches the app remain outside their control.

## Listeners

HTTP is always enabled. Listeners bind to all interfaces on their configured
port unless `--quic-host` selects a QUIC bind address. Publish only the ports
needed for a test.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_HTTP_PORT` | `--http-port` | `8080` | HTTP and WebSocket port, over TCP. |
| `ECHO_OBSERVATORY_UI` | `--ui` | `true` | Serve the embedded browser UI at `/ui/` on HTTP, HTTPS and HTTP/3. |
| `ECHO_OBSERVATORY_PUBLIC_URL` | `--public-url` | Empty | Public service base URL for canonical UI URLs, `robots.txt` and the sitemap. Empty keeps the UI unlisted. See [search discovery](usage.md#search-discovery). |
| `ECHO_OBSERVATORY_H2C` | `--h2c` | `false` | Accept cleartext HTTP/2.0 (h2c) alongside HTTP/1.1 on the HTTP port. |
| `ECHO_OBSERVATORY_TLS` | `--tls` | `false` | Enable HTTPS with HTTP/1.1 and HTTP/2.0, plus secure WebSocket connections. |
| `ECHO_OBSERVATORY_TLS_PORT` | `--tls-port` | `8443` | HTTPS port, over TCP. |
| `ECHO_OBSERVATORY_TCP` | `--tcp` | `false` | Enable the raw TCP metadata responder. |
| `ECHO_OBSERVATORY_TCP_PORT` | `--tcp-port` | `9090` | Raw TCP port. |
| `ECHO_OBSERVATORY_GRPC` | `--grpc` | `false` | Enable gRPC echo, reflection, and standard health services. |
| `ECHO_OBSERVATORY_GRPC_PORT` | `--grpc-port` | `50051` | gRPC port, over TCP. |
| `ECHO_OBSERVATORY_GRPC_TLS` | `--grpc-tls` | `false` | Use TLS on the gRPC port; requires `--grpc`. |
| `ECHO_OBSERVATORY_QUIC` | `--quic` | `false` | Enable HTTP/3 over QUIC. |
| `ECHO_OBSERVATORY_QUIC_ADVERTISE_PORT` | `--quic-advertise-port` | empty | Public UDP port advertised in HTTPS `Alt-Svc` responses. Requires HTTPS and QUIC; set only to a reachable mapped port. Deployment profiles use `443`. |
| `ECHO_OBSERVATORY_QUIC_PORT` | `--quic-port` | `4433` | HTTP/3 port, over UDP. |
| `ECHO_OBSERVATORY_QUIC_HOST` | `--quic-host` | Empty | Optional QUIC bind IP or hostname; empty binds to all available interfaces. |
| `ECHO_OBSERVATORY_UI_HTTP3_IPV4_ONLY` | `--ui-http3-ipv4-only` | `false` | Show a provider-neutral IPv4-only notice in the UI's HTTP/3 examples when the hosting provider lacks IPv6 UDP support. Informational only; does not change listener binding or IPv6 support. |
| `ECHO_OBSERVATORY_QUIC_DISABLE_PATH_MTU_DISCOVERY` | `--quic-disable-path-mtu-discovery` | `false` | Keep outgoing QUIC packets at the conservative initial size instead of increasing it after probes. Use on paths that intermittently drop larger UDP packets. |
| `ECHO_OBSERVATORY_METRICS` | `--metrics` | `true` | Enable the admin listener with `/metrics`, `/health`, and `/ready`. |
| `ECHO_OBSERVATORY_METRICS_PORT` | `--metrics-port` | `3000` | Admin HTTP port, over TCP. |

Use `--metrics=false` or `ECHO_OBSERVATORY_METRICS=false` to disable the admin listener.
WebSocket upgrades use HTTP/1.1 at `/ws`; HTTP/2 and HTTP/3 upgrades return `426`.

The public URL must be an absolute HTTP(S) URL without credentials, query,
fragment, or whitespace. A clean path prefix is allowed; a trailing slash is
added automatically. This setting controls discovery metadata only, not bind
addresses, routing, certificates, or the browser's request destination.
It is never inferred from request headers.

### Terminal example ports

The Connect page uses the hostname in the browser's address. Its listener
examples use the ports below when set, otherwise the corresponding bind ports.
Set them to your externally exposed ports when a container or Service maps
different port numbers. These settings do not change listeners or publish ports.

| Environment variable | Flag | Default |
| --- | --- | --- |
| `ECHO_OBSERVATORY_HTTP_ADVERTISE_PORT` | `--http-advertise-port` | Empty |
| `ECHO_OBSERVATORY_TLS_ADVERTISE_PORT` | `--tls-advertise-port` | Empty |
| `ECHO_OBSERVATORY_TCP_ADVERTISE_PORT` | `--tcp-advertise-port` | Empty |
| `ECHO_OBSERVATORY_GRPC_ADVERTISE_PORT` | `--grpc-advertise-port` | Empty |

HTTP/3 reuses `--quic-advertise-port`, which also controls HTTPS `Alt-Svc`.
Compose and Kubernetes advertise HTTP 80, HTTPS/HTTP3 443, TCP 9090 and
gRPC 50051. For a custom mapping such as `18080:8080`, set
`--http-advertise-port=18080`. The search-discovery `--public-url` setting does
not select these ports. The public UI does not advertise private admin settings.

## Trusted PROXY protocol

Available starting with `v2.0.0-pre.1`.

Direct connections remain the default, including on-premises installations.
Enable PROXY protocol only when a trusted TCP proxy sends a header before the
application data or TLS handshake.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_PROXY_PROTOCOL` | `--proxy-protocol` | `false` | Require PROXY protocol v1/v2 on the public HTTP, HTTPS, raw TCP and gRPC listeners. |
| `ECHO_OBSERVATORY_PROXY_TRUSTED_CIDRS` | `--proxy-trusted-cidrs` | Empty | Comma-separated CIDRs for the actual proxy socket peers allowed to provide these headers. Required when PROXY protocol is enabled. |

For example, replace these documentation addresses with your proxy's actual
source addresses:

```bash
./echo-observatory --tls --proxy-protocol \
  --proxy-trusted-cidrs='192.0.2.10/32,2001:db8:1::10/128'
```

Enabled listeners reject untrusted peers and missing or malformed PROXY headers.
Use the narrowest proxy CIDRs and restrict network access to the backend ports;
the header itself is not authentication. Trust lists with invalid CIDRs, host
bits, unspecified or multicast addresses, IPv4-mapped IPv6 prefixes, or `/0`
are rejected at startup. A trust list without `--proxy-protocol` is also an error.

Only validated address-bearing PROXY headers supply original client metadata.
The actual socket peer remains separately identifiable. `Forwarded`,
`X-Forwarded-For` and other HTTP headers do not establish a trusted client address.
Headers are consumed before TLS, so HTTPS, gRPC TLS and ACME still terminate
inside the application. Configure the upstream proxy to send the PROXY header
on every connection, including TLS-ALPN certificate validation connections.

UDP/QUIC and the private admin listener do not use PROXY protocol. Leave the
feature disabled for direct access; the Compose and Kubernetes defaults do so.

## TLS and client authentication

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_TLS_CERT_FILE` | `--tls-cert-file` | Empty | PEM server certificate chain. Supply with the private key. |
| `ECHO_OBSERVATORY_TLS_KEY_FILE` | `--tls-key-file` | Empty | PEM private key matching the server certificate. |
| `ECHO_OBSERVATORY_TLS_CLIENT_CA_FILE` | `--tls-client-ca-file` | Empty | PEM client CA bundle; require and verify client certificates. |
| `ECHO_OBSERVATORY_ACME_DOMAIN` | `--acme-domain` | Empty | Opt into Let's Encrypt for one DNS name. Requires HTTPS reachable on public TCP 443 for TLS-ALPN-01 validation. |
| `ECHO_OBSERVATORY_ACME_CACHE_DIR` | `--acme-cache-dir` | Empty | Required persistent cache for ACME account keys and certificates. Protect it as private key material; never commit its contents. |
| `ECHO_OBSERVATORY_ACME_ACCEPT_TOS` | `--acme-accept-tos` | `false` | Explicitly accept the [Let's Encrypt subscriber agreement](https://letsencrypt.org/repository/) before ACME issuance. |
| `ECHO_OBSERVATORY_ACME_STAGING` | `--acme-staging` | `false` | Test against Let's Encrypt staging, which issues untrusted certificates. Staging and production caches are isolated. |

Without custom files or ACME, encrypted listeners use a cached, in-memory self-signed
certificate for demos. Custom certificate and key files must be supplied
together, and TLS files require an enabled HTTPS, HTTP/3, or TLS-enabled gRPC
listener. Restart the application after changing certificate files.

Self-signed and supplied-file modes work without internet access, including
air-gapped installations. ACME is disabled by default and needs outbound access
to Let's Encrypt plus public DNS and inbound TCP 443. It shares a live certificate
provider across HTTPS, gRPC TLS, and HTTP/3, with automatic renewal and no restart.
Keep HTTPS reachable for future validations and preserve the writable private
cache across restarts. The cache's `production` and `staging` directories must
have mode `0700`; certificate files are written with mode `0600`.
Keep the cache outside the source checkout. If using a local `.acme/` directory,
it is excluded from Git and Docker build contexts; arbitrary cache paths are not.

ACME cannot be combined with supplied certificate files or a client CA bundle.
For private PKI or mutual TLS, use the supplied-file mode. Route TLS-ALPN
challenges to the instance managing the domain; independent replicas need
coordinated validation.

Client authentication applies to encrypted listeners only. Plain HTTP, TCP,
default plaintext gRPC, and the admin listener remain unencrypted. See
[Usage](usage.md) for trust-chain and mutual TLS examples.

## External readiness

An optional background probe contributes to readiness. Requests to `/ready` read
its cached result without making outbound calls.
Probe state logs use fixed categories; debug diagnostics may include the
configured target host or address.

| Environment variable | Flag | Default | Behavior |
| --- | --- | --- | --- |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE` | `--external-readiness-probe-type` | `none` | Probe type: `none`, `http`, `tcp`, or `icmp`. Compatibility aliases: `https` and `ping`. |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET` | `--external-readiness-probe-target` | Empty | HTTP(S) URL, TCP `host:port`, or ICMP hostname/IP. |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_INTERVAL` | `--external-readiness-probe-interval` | `10s` | Time between background checks; must be positive. |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TIMEOUT` | `--external-readiness-probe-timeout` | `2s` | Per-check deadline, including ICMP DNS resolution; must be positive. |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_METHOD` | `--external-readiness-http-method` | `GET` | Method used for HTTP readiness checks. |
| `ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS` | `--external-readiness-http-expected-status` | `200` | Expected HTTP status; accepted configuration range is 100–599. |

A probe type requires a target, and a target requires a probe type. HTTP targets
must use HTTP or HTTPS; TCP targets need a hostname and a valid numeric port.
Raw ICMP needs the explicit runtime privileges described in [Usage](usage.md).

## Startup validation

Enabled ports must be in the range 1–65535. TCP listeners cannot share a port;
HTTP/3 may use the same number because it uses UDP. The request-size limit must
be positive and the response-size limit must be between 0 and 1,048,576 bytes.
Enabled protection requires positive rates and daily allowance; the peer rate
may be zero. Invalid settings stop startup, and a listener failure terminates
the process.

Readiness requires every enabled listener to bind and any external probe to
pass. Liveness does not depend on an external service. On shutdown, readiness
becomes false before listeners drain.

## HTTP resource limits

Each HTTP, HTTPS, or HTTP/3 listener admits at most 100 running handlers,
including upgraded WebSocket sessions on HTTP/HTTPS. HTTP and HTTPS retain up
to 1,000 transport connections, cap headers at 64 KiB, and allow 100 concurrent
HTTP/2 streams per connection. For HTTP/HTTPS, headers must arrive within
5 seconds, bodies within 30 seconds, and idle connections close after
120 seconds. Busy listeners reject additional handlers with HTTP 503.

## gRPC resource limits

The gRPC listener permits 128 transport connections, 32 streams per connection,
and 100 calls decoding or running across the listener, including health watches
and reflection streams. Additional calls wait before protobuf decoding until
capacity is available or their context is cancelled; this is backpressure, not
an overload error. Incoming messages use `--max-request-size`; oversized messages
return `ResourceExhausted`. Request metadata is limited to 64 KiB. Connections
rotate after approximately 120 seconds (with 10% jitter) with a five-second grace period for active calls.
Go clients reconnect for ordinary requests; long-lived watches/streams and
incomplete calls close after the grace period and must reconnect.

## HTTP/3 resource limits

HTTP/3 permits at most 128 QUIC connections, 32 bidirectional streams per
connection, and 64 KiB of request headers. Request bodies and responses have
30-second stream deadlines after headers are decoded. Incomplete request headers
have no separate deadline.
With protection enabled, connections close after 120 seconds, including active
incomplete headers; clients reconnect as needed.
With protection enabled, QUIC uses Retry address validation and charges at least
64 ingress bytes per datagram to bound floods of tiny packets.

Path MTU discovery is enabled by default. If HTTP/3 connects but response bodies
stall on a network with a reduced or changing UDP packet limit, set
`--quic-disable-path-mtu-discovery`. This keeps outgoing packets at quic-go's
initial size (currently 1280 bytes), at the cost of some packet overhead. It does
not change HTTP/1.1, HTTP/2, TLS verification, or the transfer budgets.

## Deployment ports

Compose and Kubernetes Services expose HTTP 80/TCP, HTTPS 443/TCP,
HTTP/3 443/UDP, gRPC 50051/TCP and raw TCP 9090/TCP. Admin 3000/TCP remains
private. This does not change the bare binary's unprivileged bind defaults.
These profiles map public ports to unprivileged container listeners and need
no added capabilities.

`--quic-advertise-port=443` enables browser discovery using
`Alt-Svc: h3=":443"; ma=300` on HTTPS responses, including UI assets. It does not
open a firewall, configure DNS, or establish reachability. Leave it empty when
QUIC is not exposed, or set it to the actual public UDP port for a custom
mapping. Advertisement is off by default to avoid guessing proxy mappings.

No application flag is needed for IPv6: wildcard TCP and UDP listeners use the
host's available address families. `--quic-host=::1` restricts QUIC to IPv6
loopback when desired; a link-local bind can include its zone, such as
`--quic-host=fe80::1%eth0`, using an address/interface present on the host.
DNS, container networks, Service address families and firewalls determine
external reachability; see [IPv4 and IPv6](usage.md#ipv4-and-ipv6).
