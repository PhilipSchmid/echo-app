# Health, readiness and metrics

[README](../README.md) · [Usage](usage.md) · [Configuration](configuration.md)

The dedicated admin listener serves these endpoints. Its default port is
`3000/TCP`; keep it reachable only from your monitoring and management network.
These endpoints are documented for operators and are not advertised or fetched
by the public UI.

The [Grafana dashboard](../dashboards/README.md) provides a portable operations
view with importable JSON, query tests and a repeatable API update workflow.

| Endpoint | Purpose | Response |
| --- | --- | --- |
| `/health` | Process liveness | `200 healthy`, or `503 unhealthy` |
| `/ready` | Traffic readiness | `200 ready` after all configured listeners start and any external readiness check passes; otherwise `503` |
| `/metrics` | Prometheus scrape | Prometheus exposition with application, Go runtime and process metrics |

```bash
curl -fsS http://localhost:3000/health
curl -fsS http://localhost:3000/ready
curl -fsS http://localhost:3000/metrics
```

These are admin-port endpoints. The same path on an echo listener returns the
normal echo response. `/ready` reads cached state; scraping and probes never
trigger an outbound dependency check. External dependency failures make the app
unready without changing process liveness.

Metrics collection allows five concurrent scrapes; additional scrapes return
`503`. Collection times out after 10 seconds. A blocked collector retains its
slot until it finishes.

## Metric reference

Counters accumulate until the process restarts. Use `rate()` for throughput.
Gauges report current state. Duration histograms expose `_bucket`, `_sum` and
`_count` series, with seconds as the unit. Traffic and error series appear after
their first event.

| Metric | Type | Meaning and labels |
| --- | --- | --- |
| `echo_observatory_requests_total` | Counter | Completed HTTP requests, gRPC calls and TCP echo sessions, including unsuccessful outcomes. `listener`, `method`, `endpoint`. |
| `echo_observatory_request_duration_seconds` | Histogram | Handler duration, including unsuccessful outcomes. Same labels as requests. WebSocket and streaming RPC duration covers the entire handler/session lifetime. |
| `echo_observatory_http_responses_total` | Counter | HTTP outcome by `listener`, `endpoint`, `status_code`. Includes UI, admin, invalid requests and admission-limit responses. |
| `echo_observatory_http_response_bytes_total` | Counter | Response body bytes written by `listener`; excludes headers and WebSocket frames. |
| `echo_observatory_transport_bytes_total` | Counter | Socket bytes actually received or successfully sent. `listener`, `direction` (`received` or `sent`). Includes TLS/QUIC framing; excludes IP/TCP/UDP headers and traffic outside the app. |
| `echo_observatory_http_protocol_requests_total` | Counter | Completed HTTP requests by `listener`, actual `protocol` and client `ip_version` (socket peer by default; validated original address in PROXY mode). Distinguishes HTTP/1.0, HTTP/1.1, HTTP/2 and HTTP/3, plus IPv4/IPv6. |
| `echo_observatory_grpc_responses_total` | Counter | Final gRPC status for each completed unary or streaming call. `method`, `code`. Includes echo, health, reflection and unknown methods grouped as `other`. |
| `echo_observatory_requests_in_flight` | Gauge | Running HTTP handlers and gRPC calls by `listener`. Open WebSocket handlers and streaming RPCs remain in flight. |
| `echo_observatory_errors_total` | Counter | Actual application failures by `listener`, `error_type`, such as body-limit, read/write and panic failures. An intentional `?status=500` is an HTTP outcome, not an internal failure. |
| `echo_observatory_active_connections` | Gauge | Open transport connections by `listener`. `WebSocket` counts upgraded sessions separately. |
| `echo_observatory_connections_total` | Counter | Accepted connections by `listener`; `WebSocket` counts upgraded sessions. |
| `echo_observatory_rejections_total` | Counter | Actual rejections by `listener` and bounded `reason`: concurrency, connection, session, operation/peer rate, transfer rate, daily allowance or state failures. Waiting for capacity is backpressure, not a rejection. |
| `echo_observatory_protection_enabled` | Gauge | `1` when shared rate and transfer protection is enabled; otherwise `0`. |
| `echo_observatory_daily_egress_limit_bytes` | Gauge | Configured allowance per UTC day; `0` when protection is disabled. |
| `echo_observatory_daily_egress_reserved_bytes` | Gauge | Bytes charged to the current budget day, including durable reservations not yet sent and unused leases lost on restart. |
| `echo_observatory_daily_egress_remaining_bytes` | Gauge | Unreserved daily allowance; excludes unused bytes already in the current durable lease. |
| `echo_observatory_daily_egress_persistent` | Gauge | `1` when quota state survives restarts; otherwise `0`. |
| `echo_observatory_rate_limit_tokens_per_second` | Gauge | Configured token refill by fixed `bucket`: `operations`, `connections`, `ingress`, `egress`, `peer_operations`, `peer_connections`. Transfer buckets use bytes; other buckets use operations/connections. Peer values apply per client group. |
| `echo_observatory_rate_limit_burst_tokens` | Gauge | Maximum burst capacity for each bucket, in the same units as refill. |
| `echo_observatory_rate_limit_available_tokens` | Gauge | Refilled token availability at scrape time. Peer buckets report the minimum across groups active within the last ten minutes, without identities; they are absent when peer limiting is disabled. |
| `echo_observatory_rate_limit_consumed_tokens_total` | Counter | Global tokens charged by the four non-peer buckets. Connection tokens can be charged before peer checks; egress tokens represent conservative reservations. These are not completed requests or provider-billed bytes. |
| `echo_observatory_rate_limit_bucket_rejections_total` | Counter | Attempts rejected for insufficient global bucket tokens. Excludes peer, concurrency, daily allowance and state rejections; use `rejections_total` for the complete view. |
| `echo_observatory_rate_limit_tracked_peers` | Gauge | Recently active IPv4 address / IPv6 `/64` groups, without identity labels. |
| `echo_observatory_rate_limit_peer_capacity` | Gauge | Maximum retained client groups, currently `4096`; `0` when peer limiting is disabled. |
| `echo_observatory_tls_certificate_not_before_timestamp_seconds` | Gauge | Unix validity start of the last ordinary server leaf certificate observed. No series until observed. No certificate identity labels. |
| `echo_observatory_tls_certificate_not_after_timestamp_seconds` | Gauge | Unix expiry of that certificate. Updated when a renewed certificate is selected; excludes temporary ACME challenge certificates. |
| `echo_observatory_listener_up` | Gauge | `1` while a configured listener runs, `0` before binding or after stopping. Disabled listeners are absent. `listener`. |
| `echo_observatory_healthy` | Gauge | Process liveness, `1` or `0`, matching `/health`. |
| `echo_observatory_ready` | Gauge | Combined traffic readiness, `1` or `0`, matching `/ready`. |
| `echo_observatory_external_probe_ready` | Gauge | External dependency state, `1` for success or `0` while pending/failing. Present only when a readiness probe is configured. |
| `echo_observatory_external_probe_checks_total` | Counter | Actual background dependency checks by `result`: `success` or `failure`. |
| `echo_observatory_external_probe_duration_seconds` | Histogram | Duration of background dependency checks, in seconds. No target or error-text labels. |
| `echo_observatory_websocket_messages_total` | Counter | Complete messages received or successfully echoed. `direction` is `received` or `sent`; `message_type` is `text` or `binary`. |
| `echo_observatory_websocket_message_bytes_total` | Counter | Message payload bytes with the same labels; excludes frame overhead and control frames. |

Standard `go_*`, `process_*` and `promhttp_*` metrics cover runtime, process and
scrape behavior where supported by the platform.

## Labels and interpretation

Listener labels use the application's listener names: `HTTP`, `H2C`, `TLS`,
`QUIC`, `TCP`, `gRPC`, and `Metrics`. `WebSocket` is a separate logical session
category. A WebSocket session also has an underlying HTTP/TLS connection; do not
sum both categories as a physical connection total. HTTP/2 and HTTP/3 can carry
many concurrent requests over one connection.

Known UI pages, `/ui/config.json`, `/ui/certificate.json`, `/ws`, discovery and
admin paths have fixed route labels. UI assets share `ui_assets`. Arbitrary echo
paths and unknown methods are grouped as `other`. Query strings, addresses,
hosts, headers, payloads, probe targets and error text never become metric
labels. This keeps the number of time series bounded and avoids recording
visitor details in labels. IP-family metrics use the actual socket peer by
default, or the validated original client address in trusted PROXY protocol
mode. IPv4-mapped IPv6 addresses count as IPv4; forwarded HTTP headers do not
change these labels.

HTTP `status_code` values normally contain a numeric response code. A canceled
request or escaping panic before a final response uses `canceled` or `panic`
rather than claiming a successful `200`. A response already committed keeps its
actual status. Request totals and latency update at completion: long-lived
WebSocket and gRPC streaming calls appear there when they end, while their
current activity is visible in the in-flight and connection gauges.

HTTP measurements cover requests reaching the app's handler. Its body-byte
counter excludes headers and encryption overhead; transport bytes include socket
traffic and TLS/QUIC framing, including handshakes. Neither measures IP headers,
TCP ACKs, kernel retransmissions, pre-application drops or all provider-billed
traffic. No packet capture is performed.

Rejection reasons are `request_limit`, `connection_limit`, `session_limit`,
`request_rate`, `peer_rate`, `connection_rate`, `ingress_rate`, `egress_rate`,
`daily_egress`, `state_error` and `closed`; unknown reasons share `other`.
Rate and daily quotas are opt-in with `--protection`. Quota gauges describe
the shared guard for this process. Durable reservations
are conservative, so the reserved gauge can exceed successfully sent transport
bytes. The in-memory allowance resets on restart; durable state survives it.
Separate replicas have separate budgets.

Rate-pressure metrics are absent when protection is disabled. A scrape computes
current refill without changing admission state, performing network requests or
retaining client groups. Peer headroom is a minimum, not a sum or an identified
client. Short bursts may happen between scrapes, so compare headroom with
rejection counters. All rate-pressure labels use a fixed set of bucket names.
The configured rates and bursts make it possible to tune the existing
[protection options](configuration.md#abuse-protection);
the dashboard does not change them automatically.

Certificate dates work with ACME, supplied PEM files and the generated demo
certificate. HTTPS, TLS gRPC and HTTP/3 share the configured certificate. A
scrape only reads the observation; it never triggers issuance or renewal. No
series is emitted before a certificate is observed, and an unsuccessful
selection preserves the previous observation. Expiry alone does not prove trust,
hostname validity, chain validity or renewal success. Builds before these
metrics were introduced leave their dashboard panels empty.

gRPC call metrics cover the unary and streaming interceptors. Header validation,
malformed method names and unary message-decoding failures occur before those
interceptors and are excluded. Calls waiting for global admission are also
excluded from the in-flight gauge; waiting applies backpressure and does not
increment rejection counters.

## Useful PromQL

Completed requests per second, by listener:

```promql
sum by (listener) (rate(echo_observatory_requests_total[5m]))
```

HTTP response rates, including deliberate test statuses:

```promql
sum by (listener, status_code) (rate(echo_observatory_http_responses_total[5m]))
```

95th-percentile HTTP handler latency, excluding WebSocket session lifetimes:

```promql
histogram_quantile(0.95,
  sum by (listener, le) (
    rate(echo_observatory_request_duration_seconds_bucket{
      listener=~"HTTP|H2C|TLS|QUIC|Metrics", endpoint!="/ws"
    }[5m])
  )
)
```

Overload rejections and current readiness:

```promql
sum by (listener, reason) (rate(echo_observatory_rejections_total[5m]))
```

```promql
echo_observatory_ready == 0
```

WebSocket payload throughput in bytes per second:

```promql
sum by (direction) (rate(echo_observatory_websocket_message_bytes_total[5m]))
```

Public transport egress per listener, including TLS/QUIC framing:

```promql
sum by (listener) (rate(echo_observatory_transport_bytes_total{direction="sent"}[5m]))
```

Less than 20% of the daily allowance left, for protected instances:

```promql
echo_observatory_daily_egress_remaining_bytes
  / clamp_min(echo_observatory_daily_egress_limit_bytes, 1) < 0.2
and echo_observatory_protection_enabled == 1
```

For process availability, alert on Prometheus's own `up` scrape metric as well:
a stopped or unreachable app cannot report its own readiness gauge. For metric
design and query semantics, see the [Prometheus instrumentation guidance](https://prometheus.io/docs/practices/instrumentation/)
and [histogram guidance](https://prometheus.io/docs/practices/histograms/).
