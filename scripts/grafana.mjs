import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

export const dashboardFile = new URL('../dashboards/echo-observatory.json', import.meta.url);

export function allPanels(model) {
  return model.panels.flatMap((panel) => [panel, ...allPanels({ panels: panel.panels ?? [] })]);
}

export function renderQuery(expr, variables = {}) {
  const values = { job: '.*', instance: '.*', listener: '.*', __rate_interval: '5m', __range: '1h', ...variables };
  return expr.replace(/\$(?:\{(\w+)\}|(\w+))/g, (_, braced, bare) => {
    const name = braced ?? bare;
    if (!Object.hasOwn(values, name)) throw new Error(`Unknown dashboard variable: ${name}`);
    return values[name];
  });
}

export function prometheusRules(model) {
  return { groups: [{ name: 'dashboard-syntax-check', rules: allPanels(model).flatMap((panel) => (panel.targets ?? []).map((target) => ({ record: `dashboard_check:panel_${panel.id}_${target.refId}`, expr: renderQuery(target.expr) }))) }] };
}

export function prometheusTests(model) {
  const query = (title, target = 0) => renderQuery(allPanels(model).find((panel) => panel.title === title).targets[target].expr, { __range: '10m' });
  const labels = 'job="echo",instance="one"';
  const series = (name, values, extra = '', instance = labels) => ({ series: `${name}{${instance}${extra ? `,${extra}` : ''}}`, values });
  const tests = [{
    interval: '1m',
    input_series: [
      series('echo_observatory_ready', '1+0x20'),
      series('up', '1+0x20'),
      series('echo_observatory_ready', '1+0x20', '', 'job="echo",instance="down"'),
      series('up', '0+0x20', '', 'job="echo",instance="down"'),
      series('process_start_time_seconds', '100+0x20'),
      series('echo_observatory_requests_total', '0+6000x20', 'listener="gRPC",method="/echo.EchoService/Echo",endpoint=""'),
      series('echo_observatory_requests_total', '0+6000x20', 'listener="TCP",method="connection",endpoint=""'),
      series('echo_observatory_transport_bytes_total', '0+1000x20', 'listener="TLS",direction="sent"'),
      series('echo_observatory_transport_bytes_total', '0+1000000x20', 'listener="Metrics",direction="sent"'),
      series('echo_observatory_requests_total', '0+60x20', 'listener="TLS",method="GET",endpoint="/"'),
      series('echo_observatory_requests_total', '0+6000x20', 'listener="Metrics",method="GET",endpoint="/metrics"'),
      series('echo_observatory_active_connections', '2+0x20', 'listener="TLS"'),
      series('echo_observatory_active_connections', '2+0x20', 'listener="WebSocket"'),
      series('echo_observatory_active_connections', '1+0x20', 'listener="Metrics"'),
      ...['0.1', '1', '+Inf'].map((le) => series('echo_observatory_request_duration_seconds_bucket', '0+60x20', `listener="TLS",endpoint="/",le="${le}"`)),
      ...['0.1', '1', '+Inf'].map((le) => series('echo_observatory_request_duration_seconds_bucket', le === '0.1' ? '0+0x20' : '0+6000x20', `listener="TCP",endpoint="",le="${le}"`)),
      series('echo_observatory_protection_enabled', '1+0x20'),
      series('echo_observatory_daily_egress_remaining_bytes', '600+0x20'),
      series('echo_observatory_daily_egress_limit_bytes', '1000+0x20'),
      series('echo_observatory_protection_enabled', '0+0x20', '', 'job="echo",instance="disabled"'),
      series('echo_observatory_daily_egress_remaining_bytes', '0+0x20', '', 'job="echo",instance="disabled"'),
      series('echo_observatory_daily_egress_limit_bytes', '0+0x20', '', 'job="echo",instance="disabled"'),
      series('process_resident_memory_bytes', '25+0x20'),
      series('process_open_fds', '5+0x20'),
      series('process_max_fds', '0+0x20'),
      series('echo_observatory_rate_limit_tracked_peers', '0+0x20'),
      series('echo_observatory_rate_limit_peer_capacity', '0+0x20'),
      series('process_resident_memory_bytes', '900+0x20', '', 'job="unrelated",instance="other"'),
      series('echo_observatory_rate_limit_available_tokens', '12+0x20', 'bucket="operations"'),
      series('echo_observatory_rate_limit_burst_tokens', '120+0x20', 'bucket="operations"'),
      series('echo_observatory_tls_certificate_not_after_timestamp_seconds', '100000+0x20'),
    ],
    promql_expr_test: [
      ['Operations/s', 0, '{}', 201],
      ['Connections', 0, '{}', 2],
      ['HTTP handler latency', 1, '{}', 0.095],
      ['Readiness', 0, '{}', 0.5],
      ['Instance status', 3, `{${labels}}`, 500],
      ['HTTP routes · operations in range', 0, '{endpoint="/"}', 600],
      ['Transport data · selected range', 0, '{direction="sent"}', 10000],
      ['Process RAM', 0, '{}', 25],
      ['Burst headroom · now', 0, `{${labels},bucket="operations"}`, 0.1],
      ['Certificate validity · per instance', 2, `{${labels}}`, 99400],
    ].map(([title, target, resultLabels, value]) => ({ expr: query(title, target), eval_time: '10m', exp_samples: [{ labels: resultLabels, value }] })).concat(
      ['Peer tracking · now', 'File descriptors · now'].map((title) => ({ expr: query(title), eval_time: '10m', exp_samples: [] })),
    ),
  }];
  tests.push({ interval: '1m', input_series: [series('echo_observatory_requests_total', '0+60x5 0+60x14', 'listener="TLS",method="GET",endpoint="/"')], promql_expr_test: [{ expr: query('Operations/s'), eval_time: '11m', exp_samples: [{ labels: '{}', value: 1 }] }] });
  tests.push({ interval: '1m', input_series: [series('echo_observatory_ready', '1 stale'), series('up', '0+0x20')], promql_expr_test: [{ expr: query('Readiness'), eval_time: '6m', exp_samples: [] }] });
  tests.push({ interval: '1m', input_series: [
    series('echo_observatory_tls_certificate_not_after_timestamp_seconds', '100000+0x20'),
    series('echo_observatory_tls_certificate_not_after_timestamp_seconds', '200000+0x20', '', 'job="echo",instance="two"'),
  ], promql_expr_test: [{ expr: query('Certificate validity · per instance', 2), eval_time: '10m', exp_samples: [{ labels: `{${labels}}`, value: 99400 }, { labels: '{job="echo",instance="two"}', value: 199400 }] }] });
  return { rule_files: [], evaluation_interval: '1m', tests };
}

async function main(command) {
  const model = JSON.parse(await readFile(dashboardFile, 'utf8'));
  switch (command) {
    case 'rules':
      console.log(JSON.stringify(prometheusRules(model), null, 2));
      break;
    case 'rule-tests':
      console.log(JSON.stringify(prometheusTests(model), null, 2));
      break;
    default:
      throw new Error('Usage: node scripts/grafana.mjs rules|rule-tests');
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { await main(process.argv[2]); } catch (error) { console.error(error.message); process.exitCode = 1; }
}
