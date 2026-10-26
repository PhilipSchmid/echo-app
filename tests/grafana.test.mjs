import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dashboard } from '../dashboards/echo-observatory.mjs';
import { allPanels, dashboardFile, prometheusRules, renderQuery } from '../scripts/grafana.mjs';

const model = JSON.parse(await readFile(dashboardFile, 'utf8'));
const panels = allPanels(model);

test('committed JSON matches its source and references actual application metric families', async () => {
  assert.deepEqual(model, dashboard(), 'Run make grafana-build after editing the source');
  const sources = await Promise.all(['../internal/metrics/unified.go', '../internal/metrics/protection.go', '../internal/metrics/certificate.go', '../internal/metrics/rate_limit.go', '../internal/server/metrics_server.go'].map((path) => readFile(new URL(path, import.meta.url), 'utf8')));
  const names = [...sources.join('\n').matchAll(/"(echo_observatory_[a-z_]+)"/g)].map(([_, name]) => name);
  names.push(...[...sources.join('\n').matchAll(/desc\("([a-z_]+)"/g)].map(([_, name]) => `echo_observatory_${name}`));
  const expressions = panels.flatMap((panel) => (panel.targets ?? []).map((target) => target.expr)).join('\n');
  const available = new Set(names);
  for (const [name] of expressions.matchAll(/echo_observatory_[a-z_]+/g)) {
    const family = name.replace(/_(bucket|count|sum)$/, "");
    assert.ok(available.has(name) || available.has(family), `Unknown metric family: ${name}`);
  }
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
  const sections = [model.panels.filter((panel) => panel.type !== 'row'), ...model.panels.filter((panel) => panel.type === 'row').map((row) => row.panels)];
  for (const section of sections) for (let i = 0; i < section.length; i++) for (const other of section.slice(i + 1)) {
    const a = section[i].gridPos; const b = other.gridPos;
    assert.ok(a.x + a.w <= b.x || b.x + b.w <= a.x || a.y + a.h <= b.y || b.y + b.h <= a.y, `Overlapping panels: ${section[i].title}, ${other.title}`);
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
  for (const title of ['Instance status', 'Daily allowance', 'Configured rate & burst', 'Certificate validity · per instance']) {
    const transformations = panels.find((panel) => panel.title === title).transformations;
    assert.deepEqual(transformations[0], { id: 'filterFieldsByName', options: { exclude: { names: ['__name__'] } } });
    assert.equal(transformations[1].id, 'merge');
  }
});

test('current values and interval totals are never misleading trend graphs', () => {
  assert.equal(panels.filter((panel) => panel.type !== 'row').length, 33);
  for (const panel of panels) for (const target of panel.targets ?? []) {
    if (panel.type === 'bargauge' || panel.type === 'table' || panel.type === 'stat') assert.equal(target.instant, true, panel.title);
    if (target.expr.includes('increase(')) {
      assert.equal(target.instant, true, `Interval total must be an instant query: ${panel.title}`);
      assert.notEqual(panel.type, 'timeseries', panel.title);
    }
    if (target.expr.includes('process_start_time_seconds')) assert.equal(panel.type, 'table');
    if (target.expr.includes('tls_certificate_')) assert.equal(panel.type, 'table');
  }
  for (const panel of panels.filter((p) => p.type === 'state-timeline')) {
    assert.equal(panel.fieldConfig.defaults.custom.spanNulls, false);
    assert.equal(panel.fieldConfig.defaults.mappings[0].options['0'].text === panel.fieldConfig.defaults.mappings[0].options['1'].text, false);
  }
  for (const panel of panels.filter((p) => p.type === 'bargauge')) {
    assert.match(panel.fieldConfig.defaults.displayName, /\$\{__field.labels\.\w+\}/, 'Keep the category visible even when only one series exists');
    assert.equal(panel.options.sizing, 'manual');
    assert.ok(panel.options.maxVizHeight <= 40, 'A lone bar must not fill the panel height');
  }
  assert.equal(panels.some((p) => p.title === 'GC frequency & pause'), false);
  assert.equal(panels.some((p) => p.title === 'Certificate validity remaining'), false);
});

test('reviewed queries separate unlike protocols and invalid limit states', () => {
  const query = (title) => panels.find((panel) => panel.title === title).targets[0].expr;
  assert.match(query('HTTP routes · operations in range'), /listener=~"HTTP\|H2C\|TLS\|QUIC"/);
  assert.match(query('Readiness'), /\* on\(job, instance\).*up\{/);
  assert.match(query('File descriptors · now'), /> 0/);
  assert.match(query('Peer tracking · now'), /> 0/);
  assert.equal(panels.find((panel) => panel.title === 'HTTP outcomes').targets.length, 5, 'Status classes keep the chart bounded and readable');
});

test('dashboard updates are a manual Makefile workflow without account credentials', async () => {
  const make = await readFile(new URL('../Makefile', import.meta.url), 'utf8');
  assert.match(make, /grafana-update: grafana-build/);
  assert.match(make, /Import \(Overwrite\)/);
  assert.doesNotMatch(make, /grafana-deploy|GRAFANA_TOKEN/);
  const tooling = await readFile(new URL('../scripts/grafana.mjs', import.meta.url), 'utf8');
  assert.doesNotMatch(tooling, /GRAFANA_TOKEN|async function deploy|fetch\(/);
  await assert.rejects(readFile(new URL('../.github/workflows/dashboard.yaml', import.meta.url)), { code: 'ENOENT' });
});
