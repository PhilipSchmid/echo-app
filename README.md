# echo-app — Multi-protocol network diagnostics

[![CI](https://github.com/philipschmid/echo-app/actions/workflows/ci.yaml/badge.svg)](https://github.com/philipschmid/echo-app/actions/workflows/ci.yaml) [![Docker](https://github.com/philipschmid/echo-app/actions/workflows/docker.yaml/badge.svg)](https://github.com/philipschmid/echo-app/actions/workflows/docker.yaml) [![Release](https://github.com/philipschmid/echo-app/actions/workflows/release.yaml/badge.svg)](https://github.com/philipschmid/echo-app/actions/workflows/release.yaml)

A Go service for testing network paths, proxies, and load balancers. Connect over
HTTP, HTTPS, HTTP/3, TCP, or gRPC to inspect the receiving server and request.
WebSocket connections echo text and binary messages.

## Connect to echo-app

```mermaid
%%{init: {"themeVariables": {"fontFamily": "Arial", "edgeLabelBackground": "#f5f8fe"}, "flowchart": {"nodeSpacing": 24, "rankSpacing": 48, "padding": 18}}}%%
flowchart TB
    accTitle: echo-app connection map
    accDescr: HTTP/1.1 and HTTP/2.0 clients use HTTP port 8080 or HTTPS port 8443. Plain HTTP/2.0 requires --h2c. WebSocket uses HTTP/1.1 on either port. HTTP/3 uses UDP 4433, TCP clients use 9090, gRPC clients use 50051, and Prometheus and Kubernetes probes use the admin listener on 3000.

    web("<b>HTTP / WebSocket</b><br/>curl · browser · proxy")
    socket("<b>TCP</b><br/>nc · socket clients")
    rpc("<b>gRPC</b><br/>grpcurl · SDKs")
    ops("<b>Operations</b><br/>Prometheus · Kubernetes")

    subgraph app["echo-app"]
        http("<b>HTTP</b><br/>8080 / TCP<br/>HTTP/1.1 · HTTP/2.0<br/>/ · /ws<br/>HTTP/2.0 needs --h2c")
        https("<b>HTTPS</b><br/>8443 / TCP<br/>HTTP/1.1 · HTTP/2.0<br/>/ · /ws<br/>Optional")
        quic("<b>HTTP/3</b><br/>4433 / UDP<br/>HTTP/3 · QUIC<br/>HTTP requests only<br/>Optional")
        tcp("<b>TCP</b><br/>9090 / TCP<br/>JSON metadata<br/>Close after response<br/>Optional")
        grpc("<b>gRPC</b><br/>50051 / TCP<br/>Echo · reflection<br/>Health Check · Watch<br/>Optional")
        admin("<b>Admin</b><br/>3000 / TCP<br/>/metrics<br/>/health · /ready<br/>Enabled by default")
    end

    web --> http
    web --> https
    web -->|"HTTP/3 only"| quic
    socket --> tcp
    rpc --> grpc
    ops --> admin

    classDef client fill:#f5f8fe,stroke:#9fb5d5,color:#344e73,stroke-width:1.5px,rx:12px,ry:12px;
    classDef listener fill:#edf3ff,stroke:#8aa7d6,color:#243e69,stroke-width:1.5px,rx:12px,ry:12px;
    class web,socket,rpc,ops client;
    class http,https,quic,tcp,grpc,admin listener;
    style app fill:#fafcff,stroke:#d5e1f2,color:#263f66,stroke-width:1px,rx:16px,ry:16px;
    linkStyle default stroke:#819bc1,stroke-width:1.5px;
```

HTTP and admin listeners are enabled by default; the others are opt-in.
WebSocket upgrades use HTTP/1.1 on `/ws`. Ports are configurable.

## Quick start

Run the default HTTP and admin listeners, published only on localhost:

```bash
docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:3000:3000 \
  ghcr.io/philipschmid/echo-app:main
```

In another terminal:

```bash
curl -sS http://localhost:8080/
curl -fsS http://localhost:3000/ready
```

The image runs as UID/GID `65532`. The `main` tag follows the main branch; pin a
version or digest for repeatable deployments.

To build locally with Go 1.26 or newer:

```bash
make build-quick
./echo-app --tls --tcp --grpc --quic
```

## Example response

With `--tls --print-http-request-headers`, a locally tested request through a
reverse proxy returned this HTTPS response (abridged):

```json
{
  "hostname": "echo-app-demo",
  "listener": "TLS",
  "source_ip": "172.21.0.3",
  "http_version": "HTTP/2.0",
  "http_method": "GET",
  "http_host": "echo.example.test",
  "http_request_uri": "/checkout?test=42",
  "remote_addr": "172.21.0.3:46186",
  "local_addr": "172.21.0.2:8443",
  "tls_version": "TLS 1.3",
  "tls_server_name": "echo-app-demo",
  "tls_negotiated_protocol": "h2",
  "headers": {
    "Authorization": ["[REDACTED]"],
    "X-Forwarded-For": ["172.21.0.1"]
  }
}
```

See which backend answered, the request it received, and the protocol negotiated
on that connection. `source_ip` remains the socket peer (here, the proxy), while
the forwarded client address stays in a header and `Authorization` is redacted.

## Endpoints at a glance

| Listener | Default port | Connect to | Enable |
| --- | --- | --- | --- |
| HTTP | `8080/TCP` | JSON metadata; `/ws` for WebSocket echo | Always on; optional `--h2c` |
| HTTPS | `8443/TCP` | JSON metadata; `/ws` for WSS | `--tls` |
| HTTP/3 | `4433/UDP` | Any path except `/ws` | `--quic` |
| TCP | `9090/TCP` | One JSON metadata response followed by EOF | `--tcp` |
| gRPC | `50051/TCP` | `echo.EchoService/Echo`, standard Health, reflection | `--grpc`; optional `--grpc-tls` |
| Admin | `3000/TCP` | `/metrics`, `/health`, `/ready` | On by default; `--metrics=false` disables it |

## Version plan

Version 1.3.0 is the final planned 1.x release. The browser UI, automatic
certificate management, and related major changes are planned for 2.0.0+.

## Guides

- [Usage](docs/usage.md): connection examples, response controls,
  certificates, WebSocket, health checks, and metrics.
- [Configuration](docs/configuration.md): flags, environment variables, defaults,
  and validation.
- [Kubernetes](docs/kubernetes.md): deployment, services, probes, and pod security.
- [Development](docs/development.md): builds, tests, integration checks, and images.

Keep the admin listener private. Common credential headers are redacted by default;
`--unsafe-headers` disables that protection.

[Apache 2.0 license](LICENSE).
