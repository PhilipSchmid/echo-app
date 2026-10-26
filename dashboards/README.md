# Grafana dashboard

[README](../README.md) · [Metric reference](../docs/monitoring.md)

Import [echo-observatory.json](echo-observatory.json) into Grafana 11 or newer
and select a Prometheus datasource that scrapes Echo Observatory's private admin
listener. No extra Grafana plugins, cloud account or application changes are
needed for the existing 2.0 metrics. New certificate and rate-pressure panels
require a build containing those metrics; older builds leave them empty.

The dashboard opens with current readiness, demand, connections and memory,
plus an instance table showing scrape health, liveness and process uptime.
Expand the sections for protocols, protection, certificates, runtime resources
and optional dependency checks.

Job and Instance select the deployment. Listener filters listener-labelled
panels; gRPC outcomes, WebSocket messages, protection and runtime panels have no
listener label and retain the selected instances. Each panel explains its units
and limits in its information tooltip. **Inspect → Data** offers a table or CSV.

## Scraping

Keep port 3000 on your monitoring network. For a locally installed application
and Prometheus running on the same host:

```yaml
scrape_configs:
  - job_name: echo-observatory
    scrape_interval: 15s
    static_configs:
      - targets: ["127.0.0.1:3000"]
```

In containers, use the application's private service name instead of
`127.0.0.1`. Keep the admin listener out of public port mappings. An existing
Prometheus-compatible managed service works too.

## Update from source

Edit [echo-observatory.mjs](echo-observatory.mjs), then run:

```bash
make grafana-update
```

This regenerates the JSON, validates its structure and queries, runs calculation
fixtures, and prints the import file location. It does not contact Grafana or
change a hosted dashboard.

In Grafana, choose **New → Import**, upload `echo-observatory.json`, keep the UID
`echo-observatory`, and select **Import (Overwrite)**. Keep the existing folder
and select the same Prometheus datasource. Reusing the UID updates the existing
dashboard and preserves its URL. Move useful Grafana UI edits into the source
before the next import.

Node.js 22+ runs the generator and structural tests. Docker runs the pinned
Prometheus `promtool` image for syntax and calculation checks; no container
network access is needed after the image is available locally. Check the source
and generated JSON into Git together. CI validates them without deployment
credentials or dashboard-writing access.

For generation alone use `make grafana-build`. The individual checks are
`make test-grafana` and `make test-grafana-promql`. An offline installation can
also use Grafana's normal file provisioning with the same JSON.

## Reading the panels

- Current state belongs in cards, tables or gauges. Listener/dependency state
  uses timelines; rates, latency and resource growth use time-series charts.
  Process uptime is an elapsed value in the instance table, not a rising line.
- Operations include HTTP requests, native TCP metadata sessions and gRPC calls.
  Streams count when they finish. They are not unique visitors. HTTP route
  comparisons exclude TCP and RPC operations; arbitrary paths share `other`.
- Protocol, IP-family, RPC outcome and transferred-data bars cover the selected
  interval. They are reset-aware estimates, queried once at its end. HTTP
  outcomes use bounded status classes; deliberate 4xx/5xx tests are separate
  from recorded error events. Handler percentiles are bucket estimates, not
  network round-trip measurements, and include requested demo delays.
- Transport bytes include TLS/QUIC framing but exclude some network overhead.
  They are not a provider billing meter. Daily allowance values describe each
  process's current budget day and conservative reservations.
- Burst headroom is a current token fraction; sustained use compares charged
  token rate with configured refill. Bursts can exceed 100% refill temporarily.
  Rejections include several causes, not only rate limiting. Peer headroom is
  the lowest fraction across recently active groups, without identities.
- Certificate dates and time remaining are paired per instance. ACME challenge
  certificates are excluded. Expiry does not prove trust, complete-chain
  validity or future renewal success. Scrapes never select or issue a certificate.
- Missing samples are unavailable, not invented zeroes. Failed scrapes cannot
  count as ready; historical target discovery is limited to the selected range.
  Optional dependency metrics need a configured probe, and descriptor limits
  depend on the operating system. Runtime metrics are joined to Echo targets
  so another application's process data is excluded.

The dashboard deliberately omits duplicate traffic/quota views, mixed RPC/session
latency, GC/thread internals and separate scrape-cost charts. Those underlying
metrics remain available in the [metric reference](../docs/monitoring.md).

No operator identities, instance addresses, credentials or hosting configuration
are embedded in the dashboard. Datasource access stays controlled by Grafana.
