# Usage

[README](../README.md) · [Configuration](configuration.md) · [Kubernetes](kubernetes.md)

## Connect to the listeners

Public and on-prem profiles use the same exposed ports: HTTP 80/TCP,
HTTPS 443/TCP, HTTP/3 443/UDP, gRPC 50051/TCP, and raw TCP 9090/TCP.
Admin 3000/TCP stays private. TCP 443 and UDP 443 are separate sockets.
The UI has no separate listener.

Start the on-prem profile with `docker compose up --build -d`. It maps those
ports to unprivileged container ports, enables h2c and TLS-enabled gRPC, and
advertises HTTP/3 on UDP 443. Its default certificates work offline and are
self-signed. Stop it with `docker compose down`.

The equivalent Docker command, after building `echo-observatory:local`, is:

```bash
docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  -p 80:8080 -p 443:8443 -p 443:4433/udp \
  -p 9090:9090 -p 50051:50051 -p 127.0.0.1:3000:3000 \
  echo-observatory:local --h2c --tls --tcp --grpc --grpc-tls --quic \
  --quic-advertise-port=443
```

| Connection | Exposed address | Client example |
| --- | --- | --- |
| HTTP/1.1 | `http://localhost/` | `curl --http1.1 -sS http://localhost/` |
| HTTP/2.0 cleartext (h2c) | `http://localhost/` | `curl --http2-prior-knowledge http://localhost/` |
| HTTPS, HTTP/1.1 or HTTP/2.0 | `https://localhost/` | `curl -sk --http2 https://localhost/` |
| HTTP/3 over QUIC | `https://localhost/` | `curl -sk --http3-only https://localhost/` |
| Raw TCP | `localhost:9090` | `nc localhost 9090` |
| gRPC over TLS | `localhost:50051` | `grpcurl -insecure -d '{}' localhost:50051 echo.EchoService/Echo` |
| WebSocket | `ws://localhost/ws` | Use an HTTP/1.1 WebSocket client. |
| Secure WebSocket | `wss://localhost/ws` | Use a WebSocket client that trusts the server certificate. |

For a public deployment, replace `localhost` with your service's DNS name and
omit insecure flags once a trusted certificate is available. `-insecure` is
only for grpcurl against the self-signed local demo.

For bare-binary development, defaults remain HTTP 8080, HTTPS 8443,
HTTP/3 4433/UDP, TCP 9090, gRPC 50051 and admin 3000. HTTP and admin start by
default. Enable the others explicitly:

```bash
./echo-observatory --h2c --tls --tcp --grpc --quic --print-http-request-headers
```

HTTP supports HTTP/1.1 and, with `--h2c`, cleartext HTTP/2.0. HTTPS supports
HTTP/1.1 and HTTP/2.0. Bare-binary examples below use these internal defaults;
the deployment profiles map HTTP to 80 and HTTPS/HTTP3 to 443.

The curl HTTP/2 and HTTP/3 examples require a build with the corresponding
protocol support. `-k` skips certificate verification for the generated demo
certificate; use a trusted certificate for verified connections.
With protection enabled, HTTP/3 connections last at most 120 seconds and
validate client addresses with QUIC Retry;
incoming datagrams consume at least 64 bytes of ingress allowance, while metrics
report actual bytes.

HTTP returns metadata at arbitrary paths except `/ws` and, while the UI is enabled,
`/ui`, the `/ui/` subtree, `/robots.txt` and `/sitemap.xml`. TCP sends one JSON metadata document and closes the
connection. HTTP request bodies are consumed within the configured limit and
discarded; gRPC accepts an empty request.
These metadata responses do not echo application payloads.

## Use the browser UI

Open [http://localhost/ui/](http://localhost/ui/), or
`https://localhost/ui/`, with the deployment profile. Your page request appears
immediately, with its client address, HTTP/TLS
connection, and server-observed JSON. This is the actual navigation request,
not a second request made by the browser. Header output follows the instance
settings and redaction policy.

Below the request, the TCP/IP view groups the observed HTTP headers, TLS details,
transport ports and IP addresses by layer. Expand the application row to inspect
headers. TLS appears in a separate connection-security row. This view uses
socket metadata; it does not capture packets or expose link-layer frames.

The UI is enabled by default on each configured HTTP listener; `/` remains a
JSON metadata endpoint. HTTP, HTTPS and HTTP/3 serve the same pages and assets.
With a trusted certificate, HTTPS
responses advertise HTTP/3 on the configured public UDP port. The browser may
choose HTTP/3 on subsequent requests; it can also continue using HTTP/2.
The UI reports the protocol actually received by the application. Behind a
terminating proxy, that may differ from the browser-to-proxy protocol.

The main menu is Your request, Workbench, Examples, Connect. Open
`/ui/workbench/` to send a custom HTTP request or test WebSocket text echo;
its Browse examples link leads to `/ui/examples/`. Example links open
`/ui/workbench/?example=<id>` and fill the form; you choose when to send it.
Edit request headers as name/value rows, adding or removing rows as needed.
Blank rows are ignored and repeated names combine their values. The request and
curl preview use the same validated headers. Browsers control some headers,
including Host and Cookie.
In WebSocket echo, Enter sends and Shift+Enter adds a line break. Activity
distinguishes sent and received messages and shows their direction.
Available examples follow the instance's enabled features, including
`--response-controls` and header output. Visit `/ui/connect/` for terminal
commands and the listener reference.

The **Connection security** row shows certificate details when
HTTPS, HTTP/3 or gRPC TLS is enabled. Inspect the current server certificate:
issuer, subject, DNS/IP names, UTC validity dates, and SHA-256 fingerprint. More
details include its serial number, public key, and signature algorithm. A short
note explains client trust for self-signed, ACME staging and supplied certificates;
production Let’s Encrypt certificates normally need no custom CA. The read-only
`/ui/certificate.json` snapshot follows certificates selected by the app and
refreshes when the section is reopened. It does not trigger certificate issuance
or establish what your browser trusts. On a plain HTTP page, these fields
describe the app’s separate TLS listeners, not TLS for the current request.
Connect retains client-trust guidance and links to these insights.

Public connection examples use the page's actual origin and deployment prefix.
The Connect page offers curl and GNU wget tabs for GET, JSON POST, and HEAD,
and shows forwarded-header and response-control examples when enabled. Your
client choice applies across the page and is remembered for this browser tab's
session when storage is available. Protocols that require another client keep
their own example. Expand the listener reference for TCP, gRPC, HTTP/2 and
HTTP/3 commands. These use the current hostname and configured advertised ports,
falling back to bind ports when no override is set; see
[terminal example ports](configuration.md#terminal-example-ports) for custom mappings.
Private health, readiness and metrics remain in the
[operator reference](#health-readiness-and-metrics), outside the public UI.
The UI footer links to `/ui/privacy/` and the
[source code](https://github.com/philipschmid/echo-observatory).

Requests and WebSocket connections use the page's origin. The browser controls
protocol negotiation and restricted headers, and must trust the TLS certificate
for HTTPS and WSS. Use terminal clients for raw TCP, gRPC, and explicit protocol
selection.

Set `--ui=false` or `ECHO_OBSERVATORY_UI=false` to disable the UI. Its former paths then
return the usual metadata responses.

## Search discovery

For a public deployment, set your service's public base URL, without `/ui/`.
Replace the example domain below with your own:

```bash
ECHO_OBSERVATORY_PUBLIC_URL=https://echo.example.com/ ./echo-observatory
```

With the UI enabled, `/robots.txt` advertises `/sitemap.xml`. The sitemap lists
the five canonical pages: `/ui/`, `/ui/workbench/`, `/ui/examples/`,
`/ui/connect/`, and `/ui/privacy/`. Page metadata uses the configured public URL, without taking
hostnames from incoming headers. `/` continues returning JSON, with a `Link`
header pointing to the UI.

Diagnostic JSON and UI pages with query parameters request `noindex`.
Reflected request and connection details are excluded from search snippets.
Without a public URL, UI pages request `noindex`, `robots.txt` disallows crawling,
and the sitemap returns `404`. This is the default for local and on-prem installs;
they can opt in with their own public URL. Disabling the UI restores metadata
responses at the discovery paths too.

A deployment under `/prefix/` can set `--public-url=https://example.com/prefix/`.
Its sitemap and canonical URLs include that prefix. Serve the deployment's
`robots.txt` at the origin root, where crawlers look for it.

These are crawler hints, not access controls or a privacy guarantee. Indexing
and search-result placement are up to each search engine.

## Read request diagnostics

A plain HTTP response looks like this:

```json
{
  "timestamp": "2026-10-24T12:00:00Z",
  "hostname": "demo-host",
  "listener": "HTTP",
  "source_ip": "127.0.0.1",
  "ip_version": "IPv4",
  "http_version": "HTTP/1.1",
  "http_method": "GET",
  "http_endpoint": "/example",
  "http_host": "localhost:8080",
  "http_request_uri": "/example?item=42",
  "remote_addr": "127.0.0.1:54000",
  "local_addr": "127.0.0.1:8080",
  "local_ip_version": "IPv4"
}
```

`--message` and `--node` add environment or Kubernetes identity. HTTPS and
HTTP/3 responses also report TLS version, cipher suite, SNI, and the negotiated
application protocol. The original request URI preserves escaping and query
parameters; `http_endpoint` contains the decoded path.

`source_ip` identifies the socket peer by default. With
[trusted PROXY protocol](configuration.md#trusted-proxy-protocol) enabled, a
validated header supplies the original client address and family; the actual
transport peer is preserved in `socket_peer_ip`, `socket_peer_ip_version` and
`socket_peer_addr`. HTTP `remote_addr` and `local_addr` then describe the original
client and destination; `socket_local_addr` and `socket_local_ip_version` retain
the app's receiving socket. Forwarded HTTP headers appear in optional header
output and never establish a trusted client address.

Enable header output with `--print-http-request-headers`. Authorization and
cookie headers, and fields whose names contain secret, token, or key, are
redacted by default. `--unsafe-headers` disables this redaction only in returned
HTTP diagnostics; use it only with synthetic credentials. Server diagnostic logs
always redact common credential headers, including in unsafe mode. HTTP
diagnostic responses use `Cache-Control: no-store`.

### Privacy and logging

The app keeps no persistent request history and does not log HTTP bodies or
WebSocket messages. Routine request diagnostics require `debug` or `trace`;
they can contain client addresses, paths and headers. Common credential headers
stay redacted in logs even with `--unsafe-headers`, but custom fields can still
contain sensitive data. Use synthetic data.

The UI sets no cookies and loads fonts and assets from the same service. Only
the chosen theme and curl/wget preference enter tab session storage; request
history stays in page memory. No analytics or external IP lookup is included.

Configure application, proxy and infrastructure logging for your own installation,
including retention and who can access it. The embedded `/ui/privacy/` page
describes app behavior and labels the public demo's operator details separately.
Public self-hosted instances need a notice that matches their own operator and
data handling. The [public demo's notice](https://echo.container.education/ui/privacy/)
is available on the website.

## IPv4 and IPv6

All native listeners accept IPv4 and IPv6 when the host and deployment network
support them. The UI shows an IP-family badge next to the client address and an
IP-family row in workbench responses. JSON and gRPC responses include
`ip_version` (`IPv4` or `IPv6`); HTTP JSON also includes `local_ip_version` for
the destination, or the receiving socket without PROXY forwarding. Unavailable
or invalid addresses omit the corresponding family field. IPv4-mapped IPv6
addresses such as `::ffff:192.0.2.1` are labelled
IPv4, while scoped IPv6 addresses retain their zone identifier.

By default, these values describe the socket connection received by the app.
Proxies, NAT and Docker Desktop port forwarding can translate IPv6 to IPv4.
Trusted PROXY protocol preserves the original client's IPv4/IPv6 address and
keeps the forwarding socket separately identifiable. Ordinary forwarded HTTP
headers remain untrusted. The family describes this connection, not every
protocol the visitor's device supports; no third-party IP lookup is performed.

The Connect page offers IPv4/IPv6 curl comparisons for hostnames, with the same
service path and certificate verification. Browsers choose their address family;
use terminal clients to force one:

```bash
curl -4 --silent --show-error https://echo.container.education/
curl -6 --silent --show-error https://echo.container.education/
curl --globoff --noproxy '*' -6 'http://[::1]/'
nc -6 ::1 9090
```

The hostname must have a reachable A or AAAA address for the chosen family.
Literal IPv6 URLs and gRPC targets need brackets when including a port:
`https://[2001:db8::1]/` and `[2001:db8::1]:50051`. Certificates must cover the
DNS name or IP used by the client. Link-local addresses also need the local
interface zone; they are not internet endpoints.

Compose enables IPv6 on its user-defined network and keeps admin 3000 private
on both loopback addresses. The Docker host still needs IPv6 routing/firewall
support for external access. Kubernetes Services prefer dual-stack, falling back
to a single family when the cluster does not support both. Native HTTP/3 works
over IPv6 on compatible networks.

## Control HTTP responses

Stop any running instance, then enable the controls in one terminal:

```bash
./echo-observatory --response-controls
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
| `size` | 0–`--max-response-size` bytes; 65,536 by default | Return exactly that many zero bytes as `application/octet-stream`; absolute ceiling 1 MiB. |

Without `size`, the body remains JSON. HEAD and bodyless statuses retain their
normal HTTP semantics. Invalid, repeated, or out-of-range control parameters
return `400`. Without `--response-controls`, these parameters have no special
effect.

## Use certificates and mutual TLS

Self-signed certificates are the default for demos. For the on-prem Compose
profile, supply a trusted certificate using a local override, for example
`compose.tls.yaml`:

```yaml
services:
  echo-observatory:
    volumes:
      - ./certs:/certs:ro
    environment:
      ECHO_OBSERVATORY_TLS_CERT_FILE: /certs/server-chain.pem
      ECHO_OBSERVATORY_TLS_KEY_FILE: /certs/server-key.pem
```

Run `docker compose -f compose.yaml -f compose.tls.yaml up -d`. Provision the
files locally, make the private key readable only by the application identity
(UID 65532), and keep it out of Git and the image. The certificate must cover
the hostname visitors use. HTTPS, HTTP/3 and gRPC TLS share these files. Private
PKI works offline; clients must trust its CA. Restart after replacing PEM files.

For the bare binary, supply both a PEM certificate chain and private key.
Start a new instance in one terminal:

```bash
./echo-observatory --tls --quic --grpc --grpc-tls \
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
Each message is limited to the smallest of `--max-request-size`,
`--max-response-size` and 1 MiB; the default is 64 KiB. A response limit of zero
selects the legacy 1 MiB ceiling. Reads and writes have 30-second deadlines.
With protection enabled, each session also lasts at most 120 seconds and
receives at most 60 messages or 1 MiB of message payload. Reconnect for another
session; shared rate and transfer budgets still apply. Invalid UTF-8 text closes
with status 1007; binary messages may contain arbitrary bytes.

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

The gRPC listener exposes reflection and the standard health service. These
examples use a bare-binary listener without `--grpc-tls`; deployment profiles
use TLS instead, so remove `-plaintext` and supply the appropriate CA options.

```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -d '{"service":"echo.EchoService"}' \
  localhost:50051 grpc.health.v1.Health/Check
grpcurl -plaintext -d '{"service":"echo.EchoService"}' \
  localhost:50051 grpc.health.v1.Health/Watch
```

`Check` and `Watch` accept an empty service name or `echo.EchoService` and follow
the same readiness state. Connections rotate after approximately 120 seconds (with 10% jitter) with five seconds
to drain active calls, so long-lived watches/streams must reconnect after closure.
Ordinary Go clients reconnect automatically. Health watches also finish during
shutdown.

Metrics cover completed requests and failures, response codes, latency,
connections, overload rejections, WebSocket messages and payload bytes, and
listener/application readiness. UI and admin traffic, gRPC health and reflection,
and standard Go/process metrics are included. See the
[metric reference and PromQL examples](monitoring.md).

Keep the admin listener private and restrict diagnostic traffic with network
policies. Pin an image version or digest for repeatable deployments.

## Check an external readiness dependency

Echo Observatory makes outbound HTTP, TCP, or ICMP checks against a dependency; the
cached result contributes to `/ready`.

External checks are disabled by default. Configure a probe type and target
together; incomplete or invalid settings fail at startup. See
[configuration](configuration.md#external-readiness) for intervals, timeouts,
and expected HTTP status.

```bash
# Expect HTTP 200 within two seconds.
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE=http \
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET=https://upstream.example.com/ready \
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TIMEOUT=2s \
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_INTERVAL=10s \
./echo-observatory

# Require a TCP connection to the dependency.
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE=tcp \
ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET=db.example.com:5432 \
./echo-observatory
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
  -e ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE=icmp \
  -e ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET=127.0.0.1 \
  ghcr.io/philipschmid/echo-observatory:2.1.1
```

Use HTTP or TCP checks to retain the non-root defaults. The
[Kubernetes example](kubernetes.md) drops all capabilities; raw ICMP needs an
explicitly adjusted security context.
