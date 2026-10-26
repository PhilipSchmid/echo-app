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
  return { rule_files: [], evaluation_interval: '1m', tests: [{
    interval: '1m',
    input_series: [
      series('echo_observatory_ready', '1+0x20'),
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
      series('process_resident_memory_bytes', '900+0x20', '', 'job="unrelated",instance="other"'),
      series('echo_observatory_rate_limit_available_tokens', '12+0x20', 'bucket="operations"'),
      series('echo_observatory_rate_limit_burst_tokens', '120+0x20', 'bucket="operations"'),
      series('echo_observatory_tls_certificate_not_after_timestamp_seconds', '100000+0x20'),
    ],
    promql_expr_test: [
      ['Request rate', 0, '{}', 1],
      ['Connections', 0, '{}', 2],
      ['HTTP handler latency', 1, '{}', 0.095],
      ['Daily allowance remaining', 0, `{${labels}}`, 0.6],
      ['Process RAM', 0, '{}', 25],
      ['Operation & connection headroom', 0, `{${labels},bucket="operations"}`, 0.1],
      ['Time to expiry', 0, '{}', 99400],
    ].map(([title, target, resultLabels, value]) => ({ expr: query(title, target), eval_time: '10m', exp_samples: [{ labels: resultLabels, value }] })),
  }] };
}

export function grafanaURL(value) {
  const url = new URL(value);
  if (url.username || url.password || url.search || url.hash) throw new Error('GRAFANA_URL must not contain credentials, a query or a fragment');
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))) {
    throw new Error('Use HTTPS, or HTTP on loopback for local development');
  }
  url.pathname = `${url.pathname.replace(/\/$/, '')}/`;
  return url;
}

export async function deploy(model, { url, token, datasource, folder, orgId, fetchImpl = fetch }) {
  const base = grafanaURL(url);
  if (!token?.trim()) throw new Error('Set GRAFANA_TOKEN to a dashboard-scoped Grafana service account token');
  if (/[\x00-\x20\x7f]/.test(token)) throw new Error('GRAFANA_TOKEN contains invalid whitespace or control characters');
  if (!datasource?.trim()) throw new Error('Set GRAFANA_DATASOURCE_UID to your Prometheus datasource UID');
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  if (orgId) {
    if (!/^\d+$/.test(orgId)) throw new Error('GRAFANA_ORG_ID must be numeric');
    headers['X-Grafana-Org-Id'] = orgId;
  }
  async function request(path, options = {}) {
    const response = await fetchImpl(new URL(path, base), { ...options, headers, redirect: 'error', signal: AbortSignal.timeout(15000) });
    if (response.status === 404 && options.method !== 'POST') return null;
    // Server response bodies can contain sensitive information; report only status.
    if (!response.ok) throw new Error(`Grafana API returned HTTP ${response.status}`);
    return response.json();
  }
  const source = await request(`api/datasources/uid/${encodeURIComponent(datasource)}`);
  if (source?.type !== 'prometheus') throw new Error('Datasource is missing or is not Prometheus');
  const current = await request(`api/dashboards/uid/${encodeURIComponent(model.uid)}`);
  if (current && current.dashboard.title !== model.title) throw new Error('Dashboard UID belongs to a different dashboard; refusing to replace it');
  const next = structuredClone(model);
  next.id = current?.dashboard.id ?? null;
  next.version = current?.dashboard.version ?? 0;
  const variable = next.templating.list.find((item) => item.name === 'DS_PROMETHEUS');
  variable.current = { text: source.name, value: datasource, selected: true };
  const payload = { dashboard: next, overwrite: false, message: 'Update dashboard from version-controlled source' };
  const folderUid = folder?.trim() || current?.meta?.folderUid;
  if (folderUid) payload.folderUid = folderUid;
  const result = await request('api/dashboards/db', { method: 'POST', body: JSON.stringify(payload) });
  if (result.status !== 'success' || result.uid !== model.uid) throw new Error('Grafana did not confirm the expected dashboard');
  return { uid: result.uid, version: result.version, url: new URL(result.url.replace(/^\//, ''), base).href };
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
    case 'deploy':
      console.log(JSON.stringify(await deploy(model, { url: process.env.GRAFANA_URL, token: process.env.GRAFANA_TOKEN, datasource: process.env.GRAFANA_DATASOURCE_UID, folder: process.env.GRAFANA_FOLDER_UID, orgId: process.env.GRAFANA_ORG_ID }), null, 2));
      break;
    default:
      throw new Error('Usage: node scripts/grafana.mjs rules|rule-tests|deploy');
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { await main(process.argv[2]); } catch (error) { console.error(error.message); process.exitCode = 1; }
}
