import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dashboard } from '../dashboards/echo-observatory.mjs';
import { allPanels, dashboardFile, deploy, grafanaURL, prometheusRules, renderQuery } from '../scripts/grafana.mjs';

const model = JSON.parse(await readFile(dashboardFile, 'utf8'));
const panels = allPanels(model);

test('committed JSON matches its source and covers every application metric family', async () => {
  assert.deepEqual(model, dashboard(), 'Run make grafana-build after editing the source');
  const sources = await Promise.all(['../internal/metrics/unified.go', '../internal/metrics/protection.go', '../internal/metrics/certificate.go', '../internal/metrics/rate_limit.go', '../internal/server/metrics_server.go'].map((path) => readFile(new URL(path, import.meta.url), 'utf8')));
  const names = [...sources.join('\n').matchAll(/"(echo_observatory_[a-z_]+)"/g)].map(([_, name]) => name);
  names.push(...[...sources.join('\n').matchAll(/desc\("([a-z_]+)"/g)].map(([_, name]) => `echo_observatory_${name}`));
  const expressions = panels.flatMap((panel) => (panel.targets ?? []).map((target) => target.expr)).join('\n');
  for (const name of new Set(names)) assert.ok(expressions.includes(name), `Missing metric family: ${name}`);
});

test('panels have unique IDs, readable dimensions, units and descriptions', () => {
  assert.equal(new Set(panels.map((panel) => panel.id)).size, panels.length);
  for (const panel of panels) {
    const { x, y, w, h } = panel.gridPos;
    assert.ok(x >= 0 && y >= 0 && w > 0 && x + w <= 24 && h > 0, panel.title);
    if (panel.type === 'row') continue;
    assert.ok(panel.description?.length > 30, panel.title);
    if (panel.type !== 'text') assert.ok(panel.fieldConfig.defaults.unit, panel.title);
    assert.ok(!['piechart', 'barchart'].includes(panel.type), 'Keep time-based activity on a time axis');
    for (const target of panel.targets ?? []) assert.ok(!renderQuery(target.expr).includes('$'), panel.title);
  }
  const visible = model.panels.filter((panel) => panel.type !== 'row');
  for (let i = 0; i < visible.length; i++) for (const other of visible.slice(i + 1)) {
    const a = visible[i].gridPos; const b = other.gridPos;
    assert.ok(a.x + a.w <= b.x || b.x + b.w <= a.x || a.y + a.h <= b.y || b.y + b.h <= a.y, `Overlapping panels: ${visible[i].title}, ${other.title}`);
  }
  assert.equal(model.refresh, '30s');
});

test('dashboard is portable and never hides missing metrics with fabricated zeroes', () => {
  const json = JSON.stringify(model);
  assert.doesNotMatch(json, /fly_(?:instance|edge)|Bearer |glsa_/i);
  assert.doesNotMatch(json, /or vector\(0\)/);
  assert.ok(model.templating.list.some((variable) => variable.type === 'datasource' && variable.query === 'prometheus'));
  for (const panel of panels) for (const target of panel.targets ?? []) {
    assert.deepEqual(target.datasource, { type: 'prometheus', uid: '${DS_PROMETHEUS}' });
    assert.match(target.expr, /instance=~"\$instance"/);
    if (/\b(?:process_|go_|promhttp_|scrape_duration|up\{)/.test(target.expr)) assert.match(target.expr, /and on\(job, instance\) max_over_time\(echo_observatory_ready/);
  }
  assert.throws(() => renderQuery('$unsupported'), /Unknown/);
  assert.equal(prometheusRules(model).groups[0].rules.length, panels.reduce((count, panel) => count + (panel.targets?.length ?? 0), 0));
});

test('configuration tables remove metric identity before merging by deployment labels', () => {
  for (const title of ['Protection configuration', 'Configured rate & burst']) {
    const transformations = panels.find((panel) => panel.title === title).transformations;
    assert.deepEqual(transformations[0], { id: 'filterFieldsByName', options: { exclude: { names: ['__name__'] } } });
    assert.equal(transformations[1].id, 'merge');
  }
});

test('deployer permits TLS and local development, refuses credential-bearing or unsafe URLs', () => {
  assert.equal(grafanaURL('https://grafana.example.org/subpath').href, 'https://grafana.example.org/subpath/');
  assert.equal(grafanaURL('http://127.0.0.1:3000').protocol, 'http:');
  for (const value of ['http://grafana.example.org', 'file:///tmp/dashboard', 'https://user:password@example.org', 'https://example.org/?token=secret', 'https://example.org/#token']) assert.throws(() => grafanaURL(value));
});

function mockAPI(current = null, postStatus = 200) {
  const requests = [];
  const fetchImpl = async (url, options) => {
    requests.push({ url: String(url), options });
    assert.equal(options.redirect, 'error');
    assert.ok(options.signal);
    if (String(url).includes('/api/datasources/uid/')) return Response.json({ type: 'prometheus', name: 'Metrics' });
    if (options.method === 'POST') return Response.json({ status: 'success', uid: model.uid, version: 9, url: `/d/${model.uid}/operations` }, { status: postStatus });
    return current ? Response.json(current) : new Response(null, { status: 404 });
  };
  return { requests, fetchImpl };
}
const settings = { url: 'https://grafana.example.org/', token: 'test-token', datasource: 'metrics' };

test('deployer creates and updates the stable UID while preserving folder and revision checks', async () => {
  for (const existing of [null, { dashboard: { title: model.title, id: 7, version: 8 }, meta: { folderUid: 'operations' } }]) {
    const api = mockAPI(existing);
    const result = await deploy(model, { ...settings, folder: '', ...api });
    const body = JSON.parse(api.requests.at(-1).options.body);
    assert.equal(body.dashboard.uid, model.uid);
    assert.equal(body.dashboard.version, existing ? 8 : 0);
    assert.equal(body.overwrite, false, 'Concurrent edits must produce a conflict');
    assert.equal(body.folderUid, existing?.meta.folderUid);
    assert.equal(body.dashboard.templating.list[0].current.value, 'metrics');
    assert.equal(result.version, 9);
    assert.deepEqual(model, dashboard(), 'Deploy must not mutate the portable source');
  }
});

test('deployer fails closed on auth errors, conflicts, wrong datasources and UID collisions', async () => {
  await assert.rejects(deploy(model, { ...settings, token: '' }), /GRAFANA_TOKEN/);
  await assert.rejects(deploy(model, { ...settings, datasource: '' }), /GRAFANA_DATASOURCE_UID/);
  await assert.rejects(deploy(model, { ...settings, orgId: 'unexpected' }), /numeric/);
  const collision = mockAPI({ dashboard: { title: 'Someone else’s dashboard' } });
  await assert.rejects(deploy(model, { ...settings, ...collision }), /refusing/);
  assert.equal(collision.requests.length, 2);
  for (const status of [401, 403, 412, 500]) await assert.rejects(deploy(model, { ...settings, ...mockAPI(null, status) }), new RegExp(`HTTP ${status}`));
  await assert.rejects(deploy(model, { ...settings, fetchImpl: async () => Response.json({ type: 'loki' }) }), /not Prometheus/);
});
