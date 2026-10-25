# Upgrading to 2.0

[Back to README](../README.md) · [Configuration](configuration.md)

Version 2.0 renames the application and adds the browser UI and optional
certificate automation. Existing 1.x images and tags stay at their original
registry path.

| Setting | 1.x | 2.0+ |
| --- | --- | --- |
| Container image | `ghcr.io/philipschmid/echo-app:1.3.0` | `ghcr.io/philipschmid/echo-observatory:2.0.0-pre.0` |
| Executable | `echo-app` | `echo-observatory` |
| Go module | `github.com/PhilipSchmid/echo-app` | `github.com/PhilipSchmid/echo-observatory/v2` |
| Environment variables | `ECHO_APP_*` | `ECHO_OBSERVATORY_*` |
| Prometheus metrics | `echo_app_*` | `echo_observatory_*` |

Before switching images, update environment variables in manifests, service
definitions, and secret references, plus metric names in dashboards and alerts.
The old environment prefix is not read by 2.0. Command-line flags, listener ports,
JSON field names, and the gRPC service `echo.EchoService` keep their existing names.
Do not recreate stored credentials or certificates just to change the application name.

The browser UI is enabled by default; use `--ui=false` or
`ECHO_OBSERVATORY_UI=false` for a metadata-only service. Certificate automation
remains opt-in; self-signed and operator-supplied certificates work offline.

Incoming bodies/messages and outgoing echo responses now default to 64 KiB.
Use `--max-request-size` and `--max-response-size` when a controlled test needs
more; response output retains an absolute 1 MiB ceiling. Shared rate protection
and a 1 GiB/day egress allowance are enabled by default. Public deployments
should persist quota state; load tests may explicitly disable protection.

The [Kubernetes example](kubernetes.md) also uses new resource names and selectors.
Adapt your existing deployment rather than applying it as an in-place rename:
Kubernetes selectors cannot be changed on an existing Deployment.

To roll back, restore your previous deployment configuration, environment variable
and metric names, and a pinned 1.x image or digest. No migration changes the
published 1.x images. Keep certificate caches private and preserve them during
deployment changes.

Request counters and duration histograms now include unsuccessful outcomes and
UI/admin traffic. gRPC metrics cover health, reflection and unknown calls as well
as echo. WebSocket request duration covers the session lifetime; exclude `/ws`
when graphing ordinary HTTP latency. Update dashboards that assumed success-only
counts, and see the [monitoring reference](monitoring.md) for the added metrics.

## Deployment ports and UI routing

The 2.0 Compose and Kubernetes profiles expose HTTP 80/TCP, HTTPS 443/TCP,
HTTP/3 443/UDP, gRPC 50051/TCP and raw TCP 9090/TCP. Keep admin 3000/TCP private.
Bare-binary bind defaults remain 8080, 8443 and 4433 for HTTP, HTTPS and HTTP/3.
`/ui/` now serves the interface on HTTP/3 too; disable it across all HTTP
listeners with `--ui=false`. Ordinary paths still return JSON. Deployments that
expose QUIC set `--quic-advertise-port=443` for browser discovery. 1.x images
and their configuration are unchanged.
