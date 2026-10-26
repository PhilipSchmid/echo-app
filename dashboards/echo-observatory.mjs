// Generate the portable dashboard with: node dashboards/echo-observatory.mjs
import { writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

export function dashboard() {
  const datasource = { type: 'prometheus', uid: '${DS_PROMETHEUS}' };
  const scope = 'job=~"$job",instance=~"$instance"';
  const selector = (name, labels = '') => `${name}{${scope}${labels ? `,${labels}` : ''}}`;
  // Join runtime and scrape metrics to Echo targets, including recently failed scrapes.
  const echoTargets = `max_over_time(${selector('echo_observatory_ready')}[$__range])`;
  const metric = (name, labels = '') => name.startsWith('echo_observatory_') ? selector(name, labels) : `(${selector(name, labels)} and on(job, instance) ${echoTargets})`;
  const rate = (name, labels = '') => `(rate(${selector(name, labels)}[$__rate_interval])${name.startsWith('echo_observatory_') ? '' : ` and on(job, instance) ${echoTargets}`})`;
  const increase = (name, labels = '') => `increase(${selector(name, labels)}[$__range])`;
  const colors = ['#8AB8F8', '#83CDBB', '#C3ACEA', '#E9B989', '#E49CB3'];
  const panels = [];
  let id = 1; // Keep existing panel links stable after removing introduction panel 1.
  let y = 0;
  function panel(type, title, description, queries, x, w, h, unit = 'short', instant = false) {
    const p = {
      id: ++id, type, title, description, datasource, gridPos: { x, y, w, h },
      targets: queries.map(([expr, legendFormat = ''], i) => ({ datasource, editorMode: 'code', expr, legendFormat, instant, range: !instant, refId: String.fromCharCode(65 + i) })),
      fieldConfig: { defaults: { unit, noValue: 'No samples', color: { mode: 'palette-classic' }, mappings: [], thresholds: { mode: 'absolute', steps: [{ color: colors[0], value: null }] } }, overrides: [] },
      options: {},
    };
    panels.push(p);
    return p;
  }
  function stat(title, description, expr, x, unit = 'short', color = colors[0]) {
    const p = panel('stat', title, description, [[expr]], x, 4, 4, unit, true);
    p.fieldConfig.defaults.color = { mode: 'fixed', fixedColor: color };
    p.options = { colorMode: 'value', graphMode: 'none', textMode: 'auto', reduceOptions: { calcs: ['lastNotNull'], fields: '', values: false } };
    return p;
  }
  function chart(title, description, queries, x = 0, w = 12, unit = 'short') {
    const p = panel('timeseries', title, description, queries, x, w, 8, unit);
    p.fieldConfig.defaults.min = 0;
    p.fieldConfig.defaults.custom = { drawStyle: 'line', lineInterpolation: 'linear', lineWidth: 2, fillOpacity: 8, showPoints: 'never', spanNulls: false, axisPlacement: 'auto', stacking: { mode: 'none', group: 'A' }, scaleDistribution: { type: 'linear' } };
    p.options = { legend: { showLegend: true, displayMode: 'list', placement: 'bottom', calcs: [] }, tooltip: { mode: 'multi', sort: 'desc' } };
    return p;
  }
  function bars(title, description, expr, legend, x, w = 12, unit = 'short', bounded = false) {
    const p = panel('bargauge', title, description, [[expr, legend]], x, w, 7, unit, true);
    p.fieldConfig.defaults.min = 0;
    if (bounded) p.fieldConfig.defaults.max = 1;
    p.fieldConfig.defaults.color = { mode: 'fixed', fixedColor: colors[0] };
    // Grafana 11 hides a sole bar's name unless its display name is explicit.
    p.fieldConfig.defaults.displayName = legend.replace(/\{\{(\w+)\}\}/g, '${__field.labels.$1}');
    p.options = { orientation: 'horizontal', displayMode: 'basic', showUnfilled: true, sizing: 'manual', minVizHeight: 24, maxVizHeight: 36, minVizWidth: 0, valueMode: 'text', namePlacement: 'left', text: { titleSize: 14, valueSize: 18 }, reduceOptions: { calcs: ['lastNotNull'], fields: '', values: false } };
    return p;
  }
  function table(title, description, queries, names, x = 0, w = 24, h = 7) {
    const p = panel('table', title, description, queries, x, w, h, 'short', true);
    p.targets.forEach((t) => { t.format = 'table'; });
    p.transformations = [
      { id: 'filterFieldsByName', options: { exclude: { names: ['__name__'] } } },
      { id: 'merge', options: {} },
      { id: 'filterFieldsByName', options: { include: { names: ['job', 'instance', ...Object.keys(names)] } } },
      { id: 'organize', options: { renameByName: names } },
    ];
    p.options = { showHeader: true, cellHeight: 'sm' };
    return p;
  }
  const field = (p, name, properties) => p.fieldConfig.overrides.push({ matcher: { id: 'byName', options: name }, properties: Object.entries(properties).map(([id, value]) => ({ id, value })) });
  const boolean = (yes, no) => [{ type: 'value', options: { '0': { text: no, color: colors[4] }, '1': { text: yes, color: colors[1] } } }];
  function timeline(title, description, expr, legend, yes, no) {
    const p = panel('state-timeline', title, description, [[expr, legend]], 0, 12, 7);
    p.fieldConfig.defaults.mappings = boolean(yes, no);
    p.fieldConfig.defaults.custom = { lineWidth: 0, fillOpacity: 85, spanNulls: false };
    p.options = { mergeValues: true, showValue: 'auto', alignValue: 'center', rowHeight: 0.8, legend: { showLegend: false }, tooltip: { mode: 'single' } };
    return p;
  }
  function row(title) {
    panels.push({ id: ++id, type: 'row', title, collapsed: false, panels: [], gridPos: { x: 0, y, w: 24, h: 1 } });
    y += 1;
  }
  const listener = 'listener=~"$listener"';
  const publicListener = `${listener},listener!="Metrics"`;
  const httpListener = `${listener},listener=~"HTTP|H2C|TLS|QUIC"`;
  const ready = metric('echo_observatory_ready');
  const scrapeUp = metric('up');
  const publicRequests = rate('echo_observatory_requests_total', publicListener);
  const httpDuration = rate('echo_observatory_request_duration_seconds_bucket', `${httpListener},endpoint!="/ws"`);

  const status = stat('Readiness', 'Ready and successfully scraped targets divided by known Echo scrape targets in the selected range. Failed scrapes cannot count as ready; missing readiness samples are unavailable, never assumed healthy. See the instance table for individual states.', `sum(${ready} * on(job, instance) ${scrapeUp}) / count(${scrapeUp})`, 0, 'percentunit', colors[1]);
  status.fieldConfig.defaults.color = { mode: 'thresholds' };
  status.fieldConfig.defaults.thresholds = { mode: 'absolute', steps: [{ color: colors[4], value: null }, { color: colors[1], value: 1 }] };
  stat('Operations/s', 'Completed HTTP requests, native TCP metadata sessions and gRPC calls per second. Streams count when they finish. Includes UI traffic, excludes the admin listener; not unique visitors.', `sum(${publicRequests})`, 4, 'ops');
  stat('In-flight work', 'HTTP handlers and gRPC calls currently running, including WebSocket and RPC session lifetimes. Raw TCP sessions and the admin listener are excluded. Not a request queue length.', `sum(${metric('echo_observatory_requests_in_flight', publicListener)})`, 8);
  stat('Connections', 'Accepted transport connections currently open. WebSocket logical sessions are excluded to avoid counting their underlying HTTP/TLS connection twice. Admin connections are excluded.', `sum(${metric('echo_observatory_active_connections', `${publicListener},listener!="WebSocket"`)})`, 12);
  stat('Egress/s', 'Successfully sent socket bytes, including TLS/QUIC framing. Excludes network headers, retransmissions and infrastructure traffic; not a billing meter.', `sum(${rate('echo_observatory_transport_bytes_total', `${publicListener},direction="sent"`)})`, 16, 'Bps', colors[1]);
  stat('Process RAM', 'Current resident memory summed across selected Echo processes. Includes heap and other process memory, not total VM/container use. Listener selection does not apply.', `sum(${metric('process_resident_memory_bytes')})`, 20, 'bytes', colors[2]);
  y += 4;

  const instances = table('Instance status', 'Latest scrape, process liveness, traffic readiness and elapsed process age per target. Uptime is a current duration, not availability. Missing values are unknown; failed scrapes may leave stale process fields briefly.', [[scrapeUp], [metric('echo_observatory_healthy')], [ready], [`time() - ${metric('process_start_time_seconds')}`]], { 'Value #A': 'Scrape', 'Value #B': 'Liveness', 'Value #C': 'Readiness', 'Value #D': 'Process uptime' });
  field(instances, 'Scrape', { mappings: boolean('OK', 'Failed') });
  field(instances, 'Liveness', { mappings: boolean('Healthy', 'Unhealthy') });
  field(instances, 'Readiness', { mappings: boolean('Ready', 'Not ready') });
  field(instances, 'Process uptime', { unit: 's' });
  y += 7;
  chart('Operations by listener', 'Completed operations per second by listener. TCP metadata sessions and gRPC calls are operations, not HTTP requests. Long-lived streams appear at completion; admin excluded.', [[`sum by (listener) (${publicRequests})`, '{{listener}}']], 0, 12, 'ops');
  const latency = chart('HTTP handler latency', 'Bucket-based p50/p95 estimates for HTTP handlers. Includes deliberate demo delays and unsuccessful responses; excludes admin, raw TCP, gRPC and WebSocket session duration. Not network round-trip time; sparse traffic makes estimates coarse.', [[`histogram_quantile(0.5, sum by (le) (${httpDuration}))`, 'p50'], [`histogram_quantile(0.95, sum by (le) (${httpDuration}))`, 'p95']], 12, 12, 's');
  latency.fieldConfig.overrides.push({ matcher: { id: 'byFrameRefID', options: 'A' }, properties: [{ id: 'custom.lineStyle', value: { fill: 'dash', dash: [6, 4] } }] });
  y += 8;
  chart('HTTP outcomes', 'Completed HTTP outcomes per second. Intentional 4xx/5xx demo responses are outcomes, not proof of application failure. Requests without a final numeric status appear separately; admin excluded.', [
    ...['[12]..', '3..', '4..', '5..'].map((codes, i) => [`sum(${rate('echo_observatory_http_responses_total', `${publicListener},status_code=~"${codes}"`)})`, ['1xx / 2xx', '3xx', '4xx', '5xx'][i]]),
    [`sum(${rate('echo_observatory_http_responses_total', `${publicListener},status_code!~"[1-5][0-9][0-9]"`)})`, 'No final status / other'],
  ], 0, 12, 'ops');
  chart('Error events', 'Recorded error events per second by type, including invalid payloads, cancellations, timeouts and internal failures. These are not all server faults. One operation can produce several events; intentional status controls and admission rejections are separate. Missing series may mean no matching event yet.', [[`sum by (error_type) (${rate('echo_observatory_errors_total', publicListener)})`, '{{error_type}}']], 12, 12, 'ops');
  y += 8;

  row('01 · Protocols & traffic');
  const listeners = timeline('Listener state', 'Last reported running/stopped state by configured listener. Missing scrapes leave gaps. Disabled listeners are absent; H2C is the combined HTTP/1.1 + h2c server. Use Instance status to verify scrape freshness.', metric('echo_observatory_listener_up', listener), '{{instance}} · {{listener}}', 'Running', 'Stopped');
  listeners.gridPos.w = 24;
  y += 7;
  bars('HTTP routes · operations in range', 'Estimated completed HTTP operations over the selected interval, adjusted for counter resets. Only HTTP listeners; native TCP and gRPC empty route labels are excluded. Top eight bounded route groups; arbitrary paths share other.', `topk(8, sum by (endpoint) (${increase('echo_observatory_requests_total', httpListener)}))`, '{{endpoint}}', 0);
  bars('Transport data · selected range', 'Estimated bytes sent/received during the selected interval, adjusted for resets. An instant interval total, not a rolling-total time graph. Includes TLS/QUIC framing, excludes admin and some network overhead; not billed data.', `sum by (direction) (${increase('echo_observatory_transport_bytes_total', publicListener)})`, '{{direction}}', 12, 12, 'bytes');
  y += 7;
  bars('HTTP protocols · requests in range', 'Estimated HTTP request counts by negotiated protocol over the selected interval. Includes UI and completed WebSocket upgrades, excludes admin. Not unique connections or visitors.', `sum by (protocol) (${increase('echo_observatory_http_protocol_requests_total', publicListener)})`, '{{protocol}}', 0);
  bars('IP families · requests in range', 'Estimated HTTP request counts by observed client IP family over the selected interval. No client address labels and no unique-visitor inference. Admin excluded.', `sum by (ip_version) (${increase('echo_observatory_http_protocol_requests_total', publicListener)})`, '{{ip_version}}', 12);
  y += 7;
  bars('gRPC outcomes · calls in range', 'Estimated completed calls by final gRPC code over the selected interval, including Echo, health and reflection. Streams count when they end. Listener selection does not apply because these counters have no listener label.', `sum by (code) (${increase('echo_observatory_grpc_responses_total')})`, '{{code}}', 0);
  chart('WebSocket messages', 'Complete text/binary messages received and echoed per second; excludes control frames. Compare directions, do not add them as separate user operations. Listener selection does not apply.', [[`sum by (direction, message_type) (${rate('echo_observatory_websocket_messages_total')})`, '{{direction}} · {{message_type}}']], 12, 12, 'ops');
  y += 8;
  chart('Transport throughput', 'Socket bytes per second by direction across selected listeners, including TLS/QUIC framing. Curves show activity changes; the range-total bars answer how much data moved. Admin excluded; not billed traffic.', [[`sum by (direction) (${rate('echo_observatory_transport_bytes_total', publicListener)})`, '{{direction}}']], 0, 24, 'Bps');
  y += 8;

  row('02 · Protection & limits');
  const quota = table('Daily allowance', 'Current budget-day state per instance, not a total for the dashboard interval. Disabled protection has zero allowance. Reservations can exceed bytes actually sent and durable reservations survive restart; each replica has its own budget, not a provider billing cap.', [
    [metric('echo_observatory_protection_enabled')], [metric('echo_observatory_daily_egress_persistent')], [metric('echo_observatory_daily_egress_limit_bytes')], [metric('echo_observatory_daily_egress_reserved_bytes')], [metric('echo_observatory_daily_egress_remaining_bytes')],
  ], { 'Value #A': 'Protection', 'Value #B': 'Persistent', 'Value #C': 'Daily limit', 'Value #D': 'Reserved', 'Value #E': 'Unreserved' });
  field(quota, 'Protection', { mappings: boolean('Enabled', 'Disabled') });
  field(quota, 'Persistent', { mappings: boolean('Yes', 'No') });
  for (const name of ['Daily limit', 'Reserved', 'Unreserved']) field(quota, name, { unit: 'bytes' });
  y += 7;
  bars('Burst headroom · now', 'Fraction of tokens currently available per bucket and instance. Peer buckets show the lowest headroom across groups active in the last ten minutes, never identities. Sampled current state can miss short bursts; use rejection trends too. Absent when disabled.', `${metric('echo_observatory_rate_limit_available_tokens')} / ${metric('echo_observatory_rate_limit_burst_tokens')}`, '{{instance}} · {{bucket}}', 0, 12, 'percentunit', true);
  chart('Sustained rate use', 'Charged tokens/second divided by configured refill for each global bucket. 100% = the sustained rate; bursts can temporarily exceed it. Egress is reserved bytes, not successfully sent or billed bytes. Not a rejection probability.', [[`${rate('echo_observatory_rate_limit_consumed_tokens_total')} / ${metric('echo_observatory_rate_limit_tokens_per_second')}`, '{{instance}} · {{bucket}}']], 12, 12, 'percentunit');
  y += 8;
  chart('Admission rejections', 'Rejected work per second by reason, including rate, concurrency, quota and invalid connection-metadata checks. These are not all rate-limit events. Backpressure waits are excluded; one operation can generate more than one rejection event.', [[`sum by (reason) (${rate('echo_observatory_rejections_total', listener)})`, '{{reason}}']], 0, 12, 'ops');
  bars('Peer tracking · now', 'Recently active IPv4 address/IPv6 /64 groups divided by retained-group capacity. Near capacity, LRU eviction can reset peer burst state. No identities are exported. Absent when peer limiting is disabled.', `${metric('echo_observatory_rate_limit_tracked_peers')} / (${metric('echo_observatory_rate_limit_peer_capacity')} > 0)`, '{{instance}}', 12, 12, 'percentunit', true);
  y += 8;
  table('Configured rate & burst', 'Per instance and bucket. Refill/s and Burst are operations/connections for those buckets, bytes for ingress/egress. Peer limits apply per client group. Static configuration belongs in this table; use headroom/rate trends to tune it. Absent when disabled.', [[metric('echo_observatory_rate_limit_tokens_per_second')], [metric('echo_observatory_rate_limit_burst_tokens')]], { bucket: 'Bucket', 'Value #A': 'Refill/s', 'Value #B': 'Burst' });
  y += 7;

  row('03 · Server certificate');
  const expiry = metric('echo_observatory_tls_certificate_not_after_timestamp_seconds');
  const certificates = table('Certificate validity · per instance', 'Dates and remaining validity from the same instance and last observed ordinary server leaf certificate. Negative remaining time means expired. ACME challenges are excluded. These dates do not establish trust, hostname validity or future renewal success. Missing on older builds or before observation.', [[`${metric('echo_observatory_tls_certificate_not_before_timestamp_seconds')} * 1000`], [`${expiry} * 1000`], [`${expiry} - time()`]], { 'Value #A': 'Valid from', 'Value #B': 'Expires', 'Value #C': 'Time remaining' });
  field(certificates, 'Valid from', { unit: 'dateTimeAsIso' });
  field(certificates, 'Expires', { unit: 'dateTimeAsIso' });
  field(certificates, 'Time remaining', { unit: 's' });
  y += 7;

  row('04 · Runtime resources');
  chart('Process memory', 'Resident memory and live Go heap per instance. These overlap and must not be added. A trend helps find growth or leaks; neither measures the entire VM/container. Unrelated process metrics are excluded.', [[metric('process_resident_memory_bytes'), '{{instance}} · RSS'], [metric('go_memstats_heap_alloc_bytes'), '{{instance}} · live heap']], 0, 12, 'bytes');
  chart('Process CPU · one core = 100%', 'Process CPU fraction of one core: 100% = one full core and multi-core workloads can exceed 100%. This is not the host quota, throttle time or total VM CPU.', [[rate('process_cpu_seconds_total'), '{{instance}}']], 12, 12, 'percentunit');
  y += 8;
  bars('File descriptors · now', 'Open descriptors divided by a positive process limit, where the operating system exports it. Missing/invalid limits stay unavailable rather than becoming zero use. High sustained use can indicate connection pressure or leaked files.', `${metric('process_open_fds')} / (${metric('process_max_fds')} > 0)`, '{{instance}}', 0, 24, 'percentunit', true);
  y += 7;

  row('05 · Optional dependency checks');
  timeline('Dependency state', 'Configured external readiness dependency: Ready or Pending/failed. Not published when no dependency is configured. Scrapes and readiness requests never initiate a new outbound check.', metric('echo_observatory_external_probe_ready'), '{{instance}}', 'Ready', 'Pending / failed');
  bars('Dependency checks · selected range', 'Estimated actual background checks by result over the selected interval. These counts describe checks, not traffic or visitors. Absent when the feature has not produced a series.', `sum by (result) (${increase('echo_observatory_external_probe_checks_total')})`, '{{result}}', 12);
  y += 7;
  chart('Dependency check latency', 'Bucket-based p95 duration of actual background dependency checks, per job and instance. This trend can reveal a slow dependency. No samples when disabled or unobserved; not echo handler latency.', [[`histogram_quantile(0.95, sum by (le, job, instance) (${rate('echo_observatory_external_probe_duration_seconds_bucket')}))`, '{{job}} {{instance}}']], 0, 24, 's');

  for (const section of panels.filter((p) => p.type === 'row')) {
    const index = panels.indexOf(section);
    const next = panels.findIndex((p, i) => i > index && p.type === 'row');
    section.panels = panels.splice(index + 1, (next < 0 ? panels.length : next) - index - 1);
    section.collapsed = true;
  }
  function variable(name, label, query) {
    return { name, label, type: 'query', datasource, query: { query, refId: `variable-${name}` }, definition: query, refresh: 1, multi: true, includeAll: true, allValue: '.*', current: { selected: true, text: 'All', value: '$__all' }, options: [], sort: 1 };
  }
  return {
    uid: 'echo-observatory', title: 'Echo Observatory · Operations', description: 'Curated operational dashboard for Echo Observatory 2.x. Current state, changing rates and selected-period comparisons; standard Grafana and Prometheus only.',
    id: null, version: 1, schemaVersion: 39, editable: true, tags: ['echo-observatory', 'network', 'prometheus'], timezone: 'browser', refresh: '30s', time: { from: 'now-1h', to: 'now' }, graphTooltip: 1,
    links: [{ title: 'Metric reference', type: 'link', targetBlank: true, url: 'https://github.com/PhilipSchmid/echo-observatory/blob/main/docs/monitoring.md' }],
    templating: { list: [
      { name: 'DS_PROMETHEUS', label: 'Metrics source', type: 'datasource', query: 'prometheus', current: {}, options: [], refresh: 1 },
      variable('job', 'Job', 'label_values(echo_observatory_ready, job)'),
      variable('instance', 'Instance', 'label_values(echo_observatory_ready{job=~"$job"}, instance)'),
      variable('listener', 'Listener', `label_values({__name__=~"echo_observatory_listener_up|echo_observatory_requests_total|echo_observatory_active_connections",${scope}}, listener)`),
    ] }, panels,
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await writeFile(new URL('./echo-observatory.json', import.meta.url), `${JSON.stringify(dashboard(), null, 2)}\n`);
}
