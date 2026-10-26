// Generate the portable dashboard with: node dashboards/echo-observatory.mjs
import { writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

export function dashboard() {
  const datasource = { type: 'prometheus', uid: '${DS_PROMETHEUS}' };
  const scope = 'job=~"$job",instance=~"$instance"';
  const selector = (name, labels = '') => `${name}{${scope}${labels ? `,${labels}` : ''}}`;
  // Runtime exporters share a datasource with other apps; anchor them to Echo targets.
  const echoTargets = `max_over_time(${selector('echo_observatory_ready')}[$__range])`;
  const metric = (name, labels = '') => name.startsWith('echo_observatory_') ? selector(name, labels) : `(${selector(name, labels)} and on(job, instance) ${echoTargets})`;
  const rate = (name, labels = '') => `(rate(${selector(name, labels)}[$__rate_interval])${name.startsWith('echo_observatory_') ? '' : ` and on(job, instance) ${echoTargets}`})`;
  const sum = (expr) => `sum(${expr})`;
  const colors = ['#8AB8F8', '#83CDBB', '#C3ACEA', '#E9B989', '#E49CB3', '#A4CAD6', '#D6C98C', '#B6C0D4'];
  const panels = [];
  let id = 0;
  let y = 0;
  const target = (expr, legendFormat = '', instant = false, refId = 'A') => ({ datasource, editorMode: 'code', expr, legendFormat, instant, range: !instant, refId });
  const defaults = (unit) => ({ unit, noValue: 'No samples', color: { mode: 'palette-classic' }, mappings: [], thresholds: { mode: 'absolute', steps: [{ color: colors[0], value: null }] } });
  function panel(type, title, description, targets, x, w, h, unit = 'short', extra = {}) {
    const p = { id: ++id, type, title, description, datasource, gridPos: { x, y, w, h }, targets, fieldConfig: { defaults: defaults(unit), overrides: [] }, options: {}, ...extra };
    panels.push(p);
    return p;
  }
  function stat(title, description, expr, x, unit = 'short', color = colors[0]) {
    const p = panel('stat', title, description, [target(expr, '', true)], x, 4, 4, unit);
    p.fieldConfig.defaults.color = { mode: 'fixed', fixedColor: color };
    p.options = { colorMode: 'value', graphMode: 'none', justifyMode: 'auto', orientation: 'auto', textMode: 'auto', reduceOptions: { calcs: ['lastNotNull'], fields: '', values: false } };
    return p;
  }
  function chart(title, description, exprs, x = 0, w = 12, unit = 'short', h = 8) {
    const p = panel('timeseries', title, description, exprs.map(([expr, legend], i) => target(expr, legend, false, String.fromCharCode(65 + i))), x, w, h, unit);
    p.fieldConfig.defaults.min = 0;
    p.fieldConfig.defaults.custom = { drawStyle: 'line', lineInterpolation: 'linear', lineWidth: 2, fillOpacity: 8, showPoints: 'never', spanNulls: false, axisLabel: '', axisPlacement: 'auto', stacking: { mode: 'none', group: 'A' }, scaleDistribution: { type: 'linear' } };
    p.fieldConfig.overrides = exprs.length > 1 && exprs.every(([, legend]) => !legend.includes('{{')) ? exprs.map((_, i) => ({ matcher: { id: 'byFrameRefID', options: String.fromCharCode(65 + i) }, properties: [{ id: 'color', value: { mode: 'fixed', fixedColor: colors[i % colors.length] } }] })) : [];
    p.options = { legend: { showLegend: true, displayMode: 'table', placement: 'bottom', calcs: ['lastNotNull', 'max'] }, tooltip: { mode: 'multi', sort: 'desc' } };
    return p;
  }
  function row(title) {
    panels.push({ id: ++id, type: 'row', title, collapsed: false, panels: [], gridPos: { x: 0, y, w: 24, h: 1 } });
    y += 1;
  }
  const listener = 'listener=~"$listener"';
  const publicListener = `${listener},listener!="Metrics"`;
  const publicRequests = rate('echo_observatory_requests_total', publicListener);
  const publicHTTP = rate('echo_observatory_http_responses_total', publicListener);
  const httpDuration = rate('echo_observatory_request_duration_seconds_bucket', `${listener},listener=~"HTTP|H2C|TLS|QUIC",endpoint!="/ws"`);
  const duration = (quantile) => `histogram_quantile(${quantile}, sum by (le) (${httpDuration}))`;
  const ready = metric('echo_observatory_ready');
  const protectedMetric = (name) => `(${metric(name)} and on(job, instance) (${metric('echo_observatory_protection_enabled')} == 1))`;

  panel('text', 'Operations overview', 'Start with readiness and demand; expand protocol, protection and runtime sections for diagnostics.', [], 0, 24, 4, 'short', {
    options: { mode: 'markdown', content: '### Your service, at a glance\nHealth, demand and resource use across every listener. Filter by **Job**, **Instance** or **Listener**. Demand excludes admin traffic. Expand the sections below for protocol, protection and runtime details.\n\nUse **Inspect → Data** on any chart for a table or CSV. Empty panels show unavailable or not-yet-observed metrics, never fabricated zeroes.' },
  });
  y += 4;
  const status = stat('Readiness', 'Ready instances / selected instances with readiness samples. This is not a scrape-availability check; inspect Scrape health below.', `sum(${ready}) / count(${ready})`, 0, 'percentunit', colors[1]);
  status.fieldConfig.defaults.thresholds = { mode: 'absolute', steps: [{ color: colors[4], value: null }, { color: colors[1], value: 1 }] };
  status.fieldConfig.defaults.color = { mode: 'thresholds' };
  stat('Request rate', 'Completed echo/UI HTTP requests, TCP sessions and gRPC calls. Admin traffic excluded; long-lived sessions count when they finish. This is not a visitor count.', sum(publicRequests), 4, 'reqps');
  stat('Requests', 'Estimated increase over the selected interval, adjusted for counter resets. Streaming sessions count at completion; not unique users.', sum(`increase(${metric('echo_observatory_requests_total', publicListener)}[$__range])`), 8);
  stat('Connections', 'Physical accepted transport connections. WebSocket logical sessions excluded to avoid double counting. Admin connections excluded.', sum(metric('echo_observatory_active_connections', `${publicListener},listener!="WebSocket"`)), 12);
  stat('Egress rate', 'Successfully sent socket bytes, including TLS/QUIC framing. Not provider-billed traffic: excludes network headers, retransmissions and infrastructure traffic.', sum(rate('echo_observatory_transport_bytes_total', `${publicListener},direction="sent"`)), 16, 'Bps', colors[1]);
  stat('Process RAM', 'Resident memory summed across selected processes. Includes the Go heap and other process memory, not total VM/container usage.', sum(metric('process_resident_memory_bytes')), 20, 'bytes', colors[2]);
  y += 4;
  chart('Demand by listener', 'Completed operations per second. WebSocket and streaming RPCs count at the end of the session. Listener filter applies.', [[`sum by (listener) (${publicRequests})`, '{{listener}}']], 0, 12, 'reqps');
  chart('HTTP handler latency', 'Combined HTTP p50 / p95, in seconds. Includes configured demo delays and unsuccessful outcomes. Excludes admin, raw TCP, WebSocket sessions and gRPC lifetimes. Sparse traffic makes percentiles approximate.', [[duration(0.5), 'p50'], [duration(0.95), 'p95']], 12, 12, 's');
  panels.at(-1).fieldConfig.overrides.push({ matcher: { id: 'byFrameRefID', options: 'A' }, properties: [{ id: 'custom.lineStyle', value: { fill: 'dash', dash: [6, 4] } }] });
  y += 8;
  chart('HTTP outcomes', 'Actual HTTP statuses, including intentionally requested errors; 4xx/5xx do not necessarily indicate application faults. Canceled/panic outcomes have no final status. Admin excluded.', [[`sum by (status_code) (${publicHTTP})`, '{{status_code}}']], 0, 12, 'reqps');
  chart('Application failures & rejected work', 'Internal application failures and admission rejections are separate from deliberate test statuses. No samples means no matching event series yet. Listener filter applies.', [[`sum by (error_type) (${rate('echo_observatory_errors_total', publicListener)})`, 'Failure · {{error_type}}'], [`sum by (reason) (${rate('echo_observatory_rejections_total', publicListener)})`, 'Rejected · {{reason}}']], 12, 12, 'ops');
  y += 8;

  row('01 · Availability & connections');
  const availability = chart('Liveness & readiness', 'Per instance. 1 = healthy/ready, 0 = unhealthy/not ready. A missing scrape is a gap, not a healthy zero. Listener selection does not apply.', [[metric('echo_observatory_healthy'), '{{instance}} · live'], [ready, '{{instance}} · ready']], 0, 12, 'short', 7);
  availability.fieldConfig.defaults.max = 1;
  const up = panel('table', 'Scrape health', 'Prometheus up: 1 = last scrape succeeded, 0 = failed. Restricted to targets with Echo readiness samples in the selected range. No match is not healthy. Some managed stores do not publish scrape metadata.', [target(metric('up'), '', true)], 12, 12, 7);
  up.targets[0].format = 'table';
  up.options = { showHeader: true, cellHeight: 'sm', sortBy: [] };
  up.transformations = [{ id: 'filterFieldsByName', options: { include: { names: ['job', 'instance', 'Value'] } } }, { id: 'organize', options: { renameByName: { Value: 'Scrape status' } } }];
  up.fieldConfig.defaults.mappings = [{ type: 'value', options: { '0': { text: 'Scrape failed', color: colors[4] }, '1': { text: 'Scrape OK', color: colors[1] } } }];
  y += 7;
  const listeners = chart('Configured listener state', '1 = running, 0 = stopped. Disabled listeners have no series. H2C is the shared HTTP/1.1 + h2c server when enabled; it is not a second physical HTTP listener.', [[metric('echo_observatory_listener_up', listener), '{{instance}} · {{listener}}']], 0, 8, 'short', 7);
  listeners.fieldConfig.defaults.max = 1;
  chart('Connections & logical sessions', 'Transport connections by listener. WebSocket is a separate logical session category over HTTP/TLS: compare the lines, do not sum them.', [[`sum by (listener) (${metric('echo_observatory_active_connections', publicListener)})`, '{{listener}}']], 8, 8, 'short', 7);
  chart('Work in flight', 'Running HTTP handlers and gRPC calls, including upgraded WebSocket and streaming lifetimes. Not queued work. Admin excluded.', [[`sum by (listener) (${metric('echo_observatory_requests_in_flight', publicListener)})`, '{{listener}}']], 16, 8, 'short', 7);
  y += 7;

  row('02 · Requests, protocols & payloads');
  chart('Route activity', 'Rates by bounded route label, never raw URLs or query strings. Arbitrary echo paths share other. Includes the UI; admin excluded.', [[`sum by (endpoint) (${publicRequests})`, '{{endpoint}}']], 0, 12, 'reqps');
  chart('New connections', 'Accepted connections per second; compare transport and logical WebSocket lines independently to avoid double counting.', [[`sum by (listener) (${rate('echo_observatory_connections_total', publicListener)})`, '{{listener}}']], 12, 12, 'cps');
  y += 8;
  chart('HTTP latency by listener', 'p95 handler duration by HTTP listener. Excludes admin, raw TCP, gRPC and WebSocket session lifetimes. Includes configured demo delays.', [[`histogram_quantile(0.95, sum by (le, listener) (${httpDuration}))`, '{{listener}}']], 0, 12, 's');
  chart('TCP session duration', 'p50/p95 duration of native TCP metadata sessions. This includes writing the response and closing, not network round-trip latency.', [[`histogram_quantile(0.5, sum by (le) (${rate('echo_observatory_request_duration_seconds_bucket', `${listener},listener="TCP"`)}))`, 'p50'], [`histogram_quantile(0.95, sum by (le) (${rate('echo_observatory_request_duration_seconds_bucket', `${listener},listener="TCP"`)}))`, 'p95']], 12, 12, 's');
  y += 8;
  chart('HTTP protocol adoption', 'Completed HTTP requests by negotiated protocol. Includes UI requests, excludes admin; not unique connections.', [[`sum by (protocol) (${rate('echo_observatory_http_protocol_requests_total', publicListener)})`, '{{protocol}}']], 0, 8, 'reqps');
  chart('IPv4 & IPv6 request activity', 'HTTP requests by observed client IP family. No addresses are stored in these labels. This is request activity, not unique visitors.', [[`sum by (ip_version) (${rate('echo_observatory_http_protocol_requests_total', publicListener)})`, '{{ip_version}}']], 8, 8, 'reqps');
  chart('HTTP response bodies', 'HTTP body bytes only, excluding headers, encryption and WebSocket frames. Compare with the transport chart below; do not add the two measurements.', [[`sum by (listener) (${rate('echo_observatory_http_response_bytes_total', publicListener)})`, '{{listener}}']], 16, 8, 'Bps');
  y += 8;
  chart('Transport throughput', 'Socket traffic by listener and direction, including TLS/QUIC framing. Excludes IP/TCP/UDP headers, retransmissions and infrastructure traffic; not a billing meter.', [[`sum by (listener, direction) (${rate('echo_observatory_transport_bytes_total', publicListener)})`, '{{listener}} · {{direction}}']], 0, 12, 'Bps');
  chart('Transport data in range', 'Estimated sent / received bytes over the selected time range, adjusted for counter resets. Admin excluded; not billed data.', [[`sum by (direction) (increase(${metric('echo_observatory_transport_bytes_total', publicListener)}[$__range]))`, '{{direction}}']], 12, 12, 'bytes');
  y += 8;
  chart('gRPC outcomes', 'Completed unary/streaming RPCs by method and final status, including echo, health and reflection. Listener selection does not apply: these series have no listener label.', [[`sum by (method, code) (${rate('echo_observatory_grpc_responses_total')})`, '{{method}} · {{code}}']], 0, 12, 'ops');
  chart('gRPC call & session lifetime', 'p50/p95 seconds by RPC method. Streaming values include the entire session and are not per-message latency. Listener selection does not apply.', [[`histogram_quantile(0.5, sum by (le, method) (${rate('echo_observatory_request_duration_seconds_bucket', 'listener="gRPC"')}))`, '{{method}} · p50'], [`histogram_quantile(0.95, sum by (le, method) (${rate('echo_observatory_request_duration_seconds_bucket', 'listener="gRPC"')}))`, '{{method}} · p95']], 12, 12, 's');
  y += 8;
  chart('WebSocket messages', 'Complete text/binary messages received and echoed; control frames excluded. Listener selection does not apply: WebSocket counters have direction/type labels only.', [[`sum by (direction, message_type) (${rate('echo_observatory_websocket_messages_total')})`, '{{direction}} · {{message_type}}']], 0, 12, 'ops');
  chart('WebSocket payload throughput', 'Message payload bytes; excludes WebSocket framing and control traffic. Compare sent/received, not total transport bytes. Listener selection does not apply.', [[`sum by (direction, message_type) (${rate('echo_observatory_websocket_message_bytes_total')})`, '{{direction}} · {{message_type}}']], 12, 12, 'Bps');
  y += 8;

  row('03 · Protection & daily allowance');
  const protection = panel('table', 'Protection configuration', 'Per instance: enabled, persistent reservations and daily allowance. 0 enabled = protection disabled; 0 persistent = in-memory accounting. Job/Instance filters apply; Listener does not.', [target(metric('echo_observatory_protection_enabled'), 'Enabled', true), target(metric('echo_observatory_daily_egress_persistent'), 'Persistent', true, 'B'), target(metric('echo_observatory_daily_egress_limit_bytes'), 'Daily limit', true, 'C')], 0, 12, 7);
  protection.targets.forEach((t) => { t.format = 'table'; });
  protection.transformations = [{ id: 'filterFieldsByName', options: { exclude: { names: ['__name__'] } } }, { id: 'merge', options: {} }, { id: 'filterFieldsByName', options: { include: { names: ['job', 'instance', 'Value #A', 'Value #B', 'Value #C'] } } }, { id: 'organize', options: { renameByName: { 'Value #A': 'Enabled', 'Value #B': 'Persistent', 'Value #C': 'Daily limit' } } }];
  protection.options = { showHeader: true, cellHeight: 'sm' };
  protection.fieldConfig.overrides = [{ matcher: { id: 'byName', options: 'Daily limit' }, properties: [{ id: 'unit', value: 'bytes' }] }];
  const quota = chart('Daily allowance remaining', 'Per protected instance, for its current UTC budget day. 100% = unreserved allowance intact. Reservations can exceed bytes actually sent; restarts may lose unused leases. Each replica has its own budget. Not a provider billing cap.', [[`${protectedMetric('echo_observatory_daily_egress_remaining_bytes')} / on(job, instance) clamp_min(${metric('echo_observatory_daily_egress_limit_bytes')}, 1)`, '{{instance}}']], 12, 12, 'percentunit', 7);
  quota.fieldConfig.defaults.max = 1;
  y += 7;
  chart('Reserved & unreserved bytes', 'Current UTC budget day, not the dashboard interval. Disabled protection is omitted. Durable reservations are charged before all bytes are sent and survive restart.', [[protectedMetric('echo_observatory_daily_egress_reserved_bytes'), '{{instance}} · reserved'], [protectedMetric('echo_observatory_daily_egress_remaining_bytes'), '{{instance}} · unreserved']], 0, 12, 'bytes');
  chart('Rejected work by listener', 'Rejected operations/connections, grouped by bounded reason. No samples means no rejection series yet; waiting for capacity is backpressure, not a rejection.', [[`sum by (listener, reason) (${rate('echo_observatory_rejections_total', listener)})`, '{{listener}} · {{reason}}']], 12, 12, 'ops');
  y += 8;

  row('04 · Rate-limit pressure');
  const headroom = (buckets) => `${metric('echo_observatory_rate_limit_available_tokens', `bucket=~"${buckets}"`)} / ${metric('echo_observatory_rate_limit_burst_tokens', `bucket=~"${buckets}"`)}`;
  chart('Operation & connection headroom', 'Fraction of global burst tokens currently available. 100% = full; near 0% = little burst capacity. Sampled snapshots can miss short bursts; rejection counters retain them. Limits are per process, shared across listeners.', [[headroom('operations|connections'), '{{instance}} · {{bucket}}']], 0, 12, 'percentunit');
  panels.at(-1).fieldConfig.defaults.max = 1;
  chart('Transfer headroom', 'Remaining ingress/egress burst capacity, in percent. Refills at the configured bytes/second rate. This is short-term rate capacity, separate from the daily allowance.', [[headroom('ingress|egress'), '{{instance}} · {{bucket}}']], 12, 12, 'percentunit');
  panels.at(-1).fieldConfig.defaults.max = 1;
  y += 8;
  chart('Charged operations & sustained limits', 'Solid lines: global tokens consumed per second; dashed: configured refill. Operations include HTTP requests, RPCs and WebSocket messages. Connections are charged before expensive handshakes and may later fail peer admission. Bursts can temporarily exceed refill.', [[rate('echo_observatory_rate_limit_consumed_tokens_total', 'bucket=~"operations|connections"'), '{{instance}} · {{bucket}} · charged'], [metric('echo_observatory_rate_limit_tokens_per_second', 'bucket=~"operations|connections"'), '{{instance}} · {{bucket}} · limit']], 0, 12, 'ops');
  panels.at(-1).fieldConfig.overrides.push({ matcher: { id: 'byFrameRefID', options: 'B' }, properties: [{ id: 'custom.lineStyle', value: { fill: 'dash', dash: [6, 4] } }] });
  chart('Charged bytes & sustained limits', 'Solid: ingress received / egress reserved bytes per second. Dashed: configured refill. Reservations are conservative and can exceed successfully sent bytes. Bursts can exceed refill temporarily; not billed traffic.', [[rate('echo_observatory_rate_limit_consumed_tokens_total', 'bucket=~"ingress|egress"'), '{{instance}} · {{bucket}} · charged'], [metric('echo_observatory_rate_limit_tokens_per_second', 'bucket=~"ingress|egress"'), '{{instance}} · {{bucket}} · limit']], 12, 12, 'Bps');
  panels.at(-1).fieldConfig.overrides.push({ matcher: { id: 'byFrameRefID', options: 'B' }, properties: [{ id: 'custom.lineStyle', value: { fill: 'dash', dash: [6, 4] } }] });
  y += 8;
  chart('Peer headroom · lowest observed', 'Minimum token fraction across client groups seen in the last 10 minutes. Each line may represent a different group over time. 100% when no recent groups remain; unavailable when peer limits are disabled. No addresses or identities are exported.', [[headroom('peer_operations|peer_connections'), '{{instance}} · {{bucket}}']], 0, 12, 'percentunit');
  panels.at(-1).fieldConfig.defaults.max = 1;
  chart('Peer tracking capacity', 'Recently active IPv4 address / IPv6 /64 groups divided by the bounded 4096-entry capacity. Inactive groups are ignored at scrape time. Near capacity, LRU eviction can reset a peer burst; tune protection with this in mind.', [[`${metric('echo_observatory_rate_limit_tracked_peers')} / clamp_min(${metric('echo_observatory_rate_limit_peer_capacity')}, 1)`, '{{instance}}']], 12, 12, 'percentunit');
  panels.at(-1).fieldConfig.defaults.max = 1;
  y += 8;
  chart('Global bucket rejections', 'Insufficient global tokens, by bucket. Counts failed admission attempts, not rejected bytes. Excludes peer limits, concurrency caps and daily quota; use the rejection-reasons chart for the complete view.', [[rate('echo_observatory_rate_limit_bucket_rejections_total'), '{{instance}} · {{bucket}}']], 0, 12, 'ops');
  const limits = panel('table', 'Configured rate & burst', 'Per instance and bucket. Operations/connections use tokens; ingress/egress use bytes. Peer values apply to each client group. No rate metrics are exported when protection is disabled.', [target(metric('echo_observatory_rate_limit_tokens_per_second'), '', true), target(metric('echo_observatory_rate_limit_burst_tokens'), '', true, 'B')], 12, 12, 8);
  limits.targets.forEach((t) => { t.format = 'table'; });
  limits.transformations = [{ id: 'filterFieldsByName', options: { exclude: { names: ['__name__'] } } }, { id: 'merge', options: {} }, { id: 'filterFieldsByName', options: { include: { names: ['instance', 'job', 'bucket', 'Value #A', 'Value #B'] } } }, { id: 'organize', options: { renameByName: { 'Value #A': 'Tokens / second', 'Value #B': 'Burst tokens' } } }];
  limits.options = { showHeader: true, cellHeight: 'sm' };
  y += 8;

  row('05 · Server certificate');
  const certificateExpiry = metric('echo_observatory_tls_certificate_not_after_timestamp_seconds');
  const remaining = stat('Time to expiry', 'Minimum remaining validity across selected instances, based on the last ordinary server leaf certificate observed. Negative means expired. Not a renewal-success guarantee. No samples until a certificate is observed or when running a version without certificate metrics.', `min(${certificateExpiry} - time())`, 0, 's', colors[1]);
  remaining.gridPos.w = 8;
  remaining.fieldConfig.defaults.color = { mode: 'thresholds' };
  remaining.fieldConfig.defaults.thresholds = { mode: 'absolute', steps: [{ color: colors[4], value: null }, { color: colors[3], value: 604800 }, { color: colors[1], value: 1209600 }] };
  const expires = stat('Earliest expiry', 'Expiry date of the last observed ordinary certificate, across selected instances. Uses client-local display time. Leaf expiry alone does not verify the trust chain, hostname or renewal service.', `min(${certificateExpiry}) * 1000`, 8, 'dateTimeAsIso', colors[2]);
  expires.gridPos.w = 8;
  const validFrom = stat('Valid from', 'Most recent validity start across selected instances. A future time means the certificate is not yet valid. ACME challenge certificates are excluded.', `max(${metric('echo_observatory_tls_certificate_not_before_timestamp_seconds')}) * 1000`, 16, 'dateTimeAsIso');
  validFrom.gridPos.w = 8;
  y += 4;
  chart('Certificate validity remaining', 'Seconds to leaf expiry per instance. An upward step after a handshake is consistent with a renewed certificate being selected. The same shared certificate serves HTTPS, TLS gRPC and HTTP/3. Values do not identify the issuer and do not prove trust or future renewal.', [[`${certificateExpiry} - time()`, '{{instance}}']], 0, 24, 's');
  delete panels.at(-1).fieldConfig.defaults.min;
  y += 8;

  row('06 · Go runtime & scrape diagnostics');
  chart('Resident memory & Go heap', 'RSS, live heap and heap reserved from the OS, per process. These overlap: do not sum the lines. Does not include the host OS or other processes. Listener selection does not apply.', [[metric('process_resident_memory_bytes'), '{{instance}} · RSS'], [metric('go_memstats_heap_alloc_bytes'), '{{instance}} · live heap'], [metric('go_memstats_heap_sys_bytes'), '{{instance}} · heap reserved']], 0, 12, 'bytes');
  chart('Process CPU · one core = 100%', 'CPU fraction of one core: 100% = one full core. This is process CPU, not host CPU quota or throttling; multiple cores can exceed 100%. Listener selection does not apply.', [[rate('process_cpu_seconds_total'), '{{instance}}']], 12, 12, 'percentunit');
  y += 8;
  chart('Goroutines & OS threads', 'Counts per process. Sustained growth without matching traffic may merit investigation; no universal threshold is assumed.', [[metric('go_goroutines'), '{{instance}} · goroutines'], [metric('go_threads'), '{{instance}} · threads']], 0, 8);
  chart('GC frequency & pause', 'GC cycles per second on the left axis; mean stop-the-world pause per GC on the right. Empty pause series means no observed GC during that interval.', [[rate('go_gc_duration_seconds_count'), '{{instance}} · cycles/s'], [`${rate('go_gc_duration_seconds_sum')} / ${rate('go_gc_duration_seconds_count')}`, '{{instance}} · mean pause']], 8, 8, 'ops');
  panels.at(-1).fieldConfig.overrides.push({ matcher: { id: 'byFrameRefID', options: 'B' }, properties: [{ id: 'unit', value: 's' }, { id: 'custom.axisPlacement', value: 'right' }, { id: 'custom.lineStyle', value: { fill: 'dash', dash: [6, 4] } }] });
  chart('Process uptime', 'Seconds since process start, per instance. Resets on restart; this is not service availability over the selected period.', [[`time() - ${metric('process_start_time_seconds')}`, '{{instance}}']], 16, 8, 's');
  y += 8;
  chart('File descriptor use', 'Open descriptors divided by the process limit, where supported by the operating system. A missing series is unavailable, not zero. High sustained use can indicate connection pressure or leaked files.', [[`${metric('process_open_fds')} / clamp_min(${metric('process_max_fds')}, 1)`, '{{instance}}']], 0, 8, 'percentunit');
  chart('Scrape duration', 'Prometheus scrape duration, where the datasource publishes it. No samples means unavailable, not a zero-cost scrape. Listener selection does not apply.', [[metric('scrape_duration_seconds'), '{{instance}} · duration']], 8, 8, 's');
  chart('Exporter errors', 'Exporter metric gathering/encoding errors per second, where available. No samples is not a zero-error guarantee; also inspect Scrape health.', [[rate('promhttp_metric_handler_errors_total'), '{{instance}} · {{cause}}']], 16, 8, 'ops');
  y += 8;

  row('07 · Optional external readiness dependency');
  const external = chart('Dependency readiness', 'Only exists when an external readiness dependency is configured. 1 = ready, 0 = pending/failing. No samples normally means no dependency is configured.', [[metric('echo_observatory_external_probe_ready'), '{{instance}}']], 0, 8, 'short', 7);
  external.fieldConfig.defaults.max = 1;
  chart('Background probe results', 'Actual background checks by result. Scraping and readiness requests do not trigger additional outbound checks.', [[`sum by (result) (${rate('echo_observatory_external_probe_checks_total')})`, '{{result}}']], 8, 8, 'ops', 7);
  chart('Background probe latency', 'p95 duration of actual dependency checks, in seconds. No samples when no probe is configured or no checks have been observed.', [[`histogram_quantile(0.95, sum by (le, instance) (${rate('echo_observatory_external_probe_duration_seconds_bucket')}))`, '{{instance}}']], 16, 8, 's', 7);

  // Keep the landing view focused; Grafana expands these standard rows on demand.
  for (const title of ['02 · Requests, protocols & payloads', '03 · Protection & daily allowance', '04 · Rate-limit pressure', '05 · Server certificate', '06 · Go runtime & scrape diagnostics', '07 · Optional external readiness dependency']) {
    const index = panels.findIndex((p) => p.title === title);
    const next = panels.findIndex((p, i) => i > index && p.type === 'row');
    panels[index].panels = panels.splice(index + 1, (next < 0 ? panels.length : next) - index - 1);
    panels[index].collapsed = true;
  }
  function variable(name, label, query) {
    return { name, label, type: 'query', datasource, query: { query, refId: `variable-${name}` }, definition: query, refresh: 1, multi: true, includeAll: true, allValue: '.*', current: { selected: true, text: 'All', value: '$__all' }, options: [], sort: 1, skipUrlSync: false };
  }
  return {
    uid: 'echo-observatory', title: 'Echo Observatory · Operations', description: 'Portable operational dashboard for Echo Observatory 2.x. Standard Grafana panels and Prometheus only; no provider-specific metrics or plugins.',
    id: null, version: 1, schemaVersion: 39, editable: true, tags: ['echo-observatory', 'network', 'prometheus'], timezone: 'browser', refresh: '30s', time: { from: 'now-1h', to: 'now' }, graphTooltip: 1,
    links: [{ title: 'Metric reference', type: 'link', targetBlank: true, url: 'https://github.com/PhilipSchmid/echo-observatory/blob/main/docs/monitoring.md' }],
    templating: { list: [
      { name: 'DS_PROMETHEUS', label: 'Metrics source', type: 'datasource', query: 'prometheus', current: {}, options: [], refresh: 1 },
      variable('job', 'Job', 'label_values(echo_observatory_ready, job)'),
      variable('instance', 'Instance', 'label_values(echo_observatory_ready{job=~"$job"}, instance)'),
      variable('listener', 'Listener', `label_values({__name__=~"echo_observatory_listener_up|echo_observatory_requests_total|echo_observatory_active_connections",${scope}}, listener)`),
    ] },
    panels,
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await writeFile(new URL('./echo-observatory.json', import.meta.url), `${JSON.stringify(dashboard(), null, 2)}\n`);
}
