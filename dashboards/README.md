# Grafana dashboard

[README](../README.md) · [Metric reference](../docs/monitoring.md)

Import [echo-observatory.json](echo-observatory.json) into Grafana 11 or newer
and select a Prometheus datasource that scrapes Echo Observatory's private admin
listener. No extra Grafana plugins, cloud account or application changes are
needed for the existing 2.0 metrics. New certificate and rate-pressure panels
require a build containing those metrics; older builds leave them empty.

The dashboard opens with readiness, request activity, connections, traffic and
process memory. Expand the sections for protocol breakdowns, rate limiting,
certificate validity, runtime diagnostics and optional dependency checks.
Job and Instance select the deployment. Listener filters listener-labelled
charts; gRPC outcome, WebSocket message, protection and runtime panels have no
listener label and retain the selected instances. Every panel explains its
units and limits in its information tooltip. Grafana's **Inspect → Data** offers
a table and CSV export.

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

Edit [echo-observatory.mjs](echo-observatory.mjs), regenerate the importable JSON,
and run the checks:

```bash
make grafana-build
make test-grafana test-grafana-promql
```

The source uses small helpers to keep units, spacing and panel options
consistent. Node.js 22+ runs the generator and tests. Docker runs the pinned
Prometheus `promtool` image for syntax checks and calculation fixtures; it needs
no network access after the image is available locally. Normal CI runs both.
Check generated JSON into Git alongside the source.

For API updates, set `GRAFANA_URL`, `GRAFANA_DATASOURCE_UID` and `GRAFANA_TOKEN`
in your environment or secret manager, then run:

```bash
make grafana-deploy
```

Use a Grafana service account with permission to read the selected datasource
and read/write this dashboard or its folder. Do not store its token in Git or
pass it as a command-line argument. HTTPS is required except on loopback for
local development; normal TLS verification stays enabled.

The updater uses the stable UID `echo-observatory`, preserves the existing folder
and checks the saved revision. Concurrent edits return a conflict rather than
being overwritten silently. Re-running updates the same dashboard. Set
`GRAFANA_FOLDER_UID` to choose a folder explicitly, and `GRAFANA_ORG_ID` if your
Grafana API authentication needs an organization selector. Keep local UI edits
in the source before the next update.

The **Update Grafana dashboard** GitHub Actions workflow provides the same update
as a manual action. Configure a `grafana` environment with the variables above
and `GRAFANA_TOKEN` as an environment secret. Add required reviewers if updates
need approval. It validates before deploying and never runs on pull requests.
Some managed Grafana accounts allow dashboard imports but do not expose service
account tokens. Use JSON import there; the API workflow needs an authorized token
and does not extract or reuse browser session cookies.
An offline installation can provision the JSON through Grafana's normal file
provisioning instead; no API token or GitHub Actions is required.

## Reading the panels

- Request counts describe operations, not unique visitors. Streaming sessions
  count when they finish. Admin traffic is excluded from demand panels.
- HTTP status codes include intentional test responses. Use application
  failures and rejection reasons to investigate actual faults or admission
  limits. HTTP latency includes requested demo delays; gRPC and TCP durations
  are shown separately.
- Transport bytes include TLS/QUIC framing but exclude some network overhead.
  They are not a provider billing meter. Daily quota panels describe each
  process's conservative reservations for its current budget day.
- Full token buckets mean burst headroom is available. Low headroom and rising
  rejection counters indicate pressure. A short burst may disappear between
  scrapes; use both charts. Charged-token rates can exceed sustained refill
  temporarily because bursts are allowed. Peer headroom is the lowest remaining
  fraction across recently active groups, without exporting identities.
- Certificate panels show the last ordinary server leaf certificate observed.
  ACME challenges are ignored. An expiry increase after selection can indicate
  renewal, but these metrics do not prove issuer trust, a complete chain or
  future renewal success. No certificate selection happens during a scrape.
- Empty panels distinguish unavailable or unobserved series from zero. Optional
  dependency metrics need a configured probe; scrape metadata and file
  descriptors depend on the datasource and operating system. Runtime series are
  joined to Echo targets so another application's process metrics are excluded.

The portable dashboard includes no operator identities, instance addresses,
credentials or hosting configuration. Datasource access remains controlled by
your Grafana installation.
