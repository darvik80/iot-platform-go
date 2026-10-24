import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  AGGREGATIONS, CHANNEL_KINDS, MAX_SOURCES, MAX_WIDGETS, RULE_KINDS, WINDOWS, WIDGET_TYPES,
  acceptsMetric, accountRequest, channelPayload, chartDomain, createWidget, formatValue,
  gaugePercent, latestPoint, lineSegments, monitoringPermissions, moveWidget, newWidgetID, numeric,
  sourceId, validateChannel, validateRule, validateSources, validateWidget,
} from '../../api/monitoring.ts';
import { en } from '../../i18n/en.ts';
import { ru } from '../../i18n/ru.ts';

const source = (device_id = 1, key = 'temperature') => ({ device_id, key, label: `Device ${device_id}`, color: '#3d9cf0' });
const widget = (patch = {}) => ({ ...createWidget('widget-1'), title: 'Temperature', sources: [source()], ...patch });
const kinds = { [sourceId(source())]: 'number' };
const rule = (patch = {}) => ({ name: 'High temperature', device_id: 1, metric: 'temperature', kind: 'threshold', operator: 'gt', threshold: 80, recovery: 75, for_seconds: 30, missing_seconds: 300, cooldown_seconds: 60, enabled: true, channel_ids: [], ...patch });
const result = (points) => ({ from: 0, to: 1000, series: [{ ...source(), points }] });

// No snapshots: exercise boundary values, compatibility and account-switch races.
test('widget defaults match the exact write contract and registry', () => {
  assert.deepEqual(Object.keys(createWidget('id')).sort(), ['id', 'title', 'type', 'sources', 'window_seconds', 'aggregation', 'width', 'min', 'max'].sort());
  assert.equal(MAX_SOURCES, 8);
  assert.equal(MAX_WIDGETS, 24);
  assert.equal(validateWidget(widget(), kinds), '');
  assert.ok(WIDGET_TYPES[createWidget('id').type]);
});
test('widget IDs work without the secure-context randomUUID API', () => {
  const original = globalThis.crypto;
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: { getRandomValues: original.getRandomValues.bind(original) } });
  try {
    const ids = Array.from({ length: 100 }, () => newWidgetID());
    assert.equal(new Set(ids).size, 100);
    for (const id of ids) assert.match(id, /^[a-f0-9]{32}$/);
  } finally { Object.defineProperty(globalThis, 'crypto', { configurable: true, value: original }); }
});
test('sources support multiple devices but not duplicates or more than eight', () => {
  assert.equal(validateSources([source(), source(2)]), '');
  assert.match(validateSources([source(), source()]), /duplicate$/);
  assert.match(validateSources([]), /sources$/);
  assert.equal(validateSources(Array.from({ length: 8 }, (_, i) => source(i + 1))), '');
  assert.match(validateSources(Array.from({ length: 9 }, (_, i) => source(i + 1))), /sources$/);
  for (const id of [0, -1, NaN, 1.5]) assert.notEqual(validateSources([source(id)]), '');
  assert.notEqual(sourceId(source(1, '2:3')), sourceId(source(12, ':3')));
});
test('registry controls numeric versus textual and boolean compatibility', () => {
  for (const [type, config] of Object.entries(WIDGET_TYPES)) {
    for (const aggregation of AGGREGATIONS) {
      assert.equal(acceptsMetric(type, aggregation, 'number'), true);
      for (const kind of ['text', 'boolean']) assert.equal(acceptsMetric(type, aggregation, kind), !config.numericOnly && aggregation === 'last');
    }
  }
  assert.match(validateWidget(widget(), { [sourceId(source())]: 'text' }), /numeric$/);
  assert.match(validateWidget(widget(), {}), /numeric$/);
  assert.equal(validateWidget(widget({ type: 'table', aggregation: 'last' }), { [sourceId(source())]: 'text' }), '');
});
test('wizard catches invalid ranges, names, colors and windows without converting blanks to zero', () => {
  for (const [patch, suffix] of [
    [{ title: '   ' }, 'title'], [{ window_seconds: 0 }, 'window'], [{ window_seconds: 1.5 }, 'window'],
    [{ window_seconds: 59 }, 'window'], [{ window_seconds: 31 * 86400 + 1 }, 'window'],
    [{ width: 5 }, 'width'], [{ type: 'gauge', min: 10, max: 10 }, 'range'],
    [{ type: 'gauge', min: 11, max: 10 }, 'range'], [{ min: NaN }, 'range'],
    [{ sources: [{ ...source(), color: 'red' }] }, 'color'], [{ type: 'unsupported' }, 'type'],
  ]) assert.ok(validateWidget(widget(patch), kinds).endsWith(suffix));
  assert.equal(validateWidget(widget({ type: 'gauge', min: -20, max: 0 }), kinds), '');
});
test('moving widgets is immutable, keeps IDs and respects boundaries', () => {
  const first = widget(), second = widget({ id: 'second' }), third = widget({ id: 'third' });
  const original = [first, second, third];
  assert.deepEqual(moveWidget(original, 0, 1), [second, first, third]);
  assert.deepEqual(moveWidget(original, 2, -1), [first, third, second]);
  assert.deepEqual(moveWidget(original, 0, -1), original);
  assert.deepEqual(moveWidget(original, 3, -1), original);
  assert.deepEqual(original, [first, second, third]);
});
test('formatting preserves zero, false and text, and never turns missing values into zero', () => {
  const fmt = value => formatValue(value, 'en', 'No data', 'Yes', 'No');
  for (const value of [undefined, null, NaN, Infinity]) assert.equal(fmt(value), 'No data');
  assert.equal(fmt(0), '0');
  assert.equal(fmt(false), 'No');
  assert.equal(fmt(true), 'Yes');
  assert.equal(fmt('offline'), 'offline');
  assert.equal(fmt(''), '');
  assert.equal(numeric(false), false);
  assert.equal(numeric('12'), false);
  assert.equal(numeric(0), true);
});
test('latest point is chronological, immutable, and preserves a latest explicit null', () => {
  const points = [{ ts: 3, value: null }, { ts: 1, value: 20 }, { ts: 2, value: 10 }];
  assert.deepEqual(latestPoint(points), points[0]);
  assert.equal(latestPoint([]), undefined);
  assert.deepEqual(points.map(p => p.ts), [3, 1, 2]);
});
test('gauge clamps outliers but distinguishes no data from the lower bound', () => {
  assert.equal(gaugePercent(0, 0, 100), 0);
  assert.equal(gaugePercent(-40, -20, 20), 0);
  assert.equal(gaugePercent(40, -20, 20), 100);
  assert.equal(gaugePercent(0, -20, 20), 50);
  for (const value of [null, undefined, false, '0', NaN]) assert.equal(gaugePercent(value, 0, 100), null);
  assert.equal(gaugePercent(1, 10, 10), null);
  assert.equal(gaugePercent(1, 20, 10), null);
});
test('chart uses full query time range, expands constant values and zero-bases bars', () => {
  assert.equal(chartDomain(result([{ ts: 1, value: null }])), null);
  const constant = chartDomain(result([{ ts: 900, value: 10 }]));
  assert.equal(constant.from, 0); assert.equal(constant.to, 1000);
  assert.ok(constant.min < 10 && constant.max > 10);
  assert.deepEqual(chartDomain(result([{ ts: 2, value: 5 }, { ts: 3, value: 10 }]), true), { min: 0, max: 10, from: 0, to: 1000 });
  assert.equal(chartDomain(result([{ ts: 2, value: -5 }]), true).max, 0);
  assert.ok(chartDomain({ ...result([{ ts: 2, value: 0 }]), from: 1, to: 1 }).to > 1);
});
test('line segments preserve gaps, discard nonnumeric points, and sort without mutation', () => {
  const points = [{ ts: 4, value: 4 }, { ts: 1, value: 0 }, { ts: 2, value: null }, { ts: 3, value: false }];
  assert.deepEqual(lineSegments(points), [[{ ts: 1, value: 0 }], [{ ts: 4, value: 4 }]]);
  assert.equal(points[0].ts, 4);
});
test('role and suspension gates do not grant writes to viewers or suspended admins', () => {
  assert.deepEqual(monitoringPermissions(null), { readChannels: false, writeRules: false, writeDashboards: false });
  assert.deepEqual(monitoringPermissions({ role: 'viewer' }), { readChannels: false, writeRules: false, writeDashboards: false });
  assert.deepEqual(monitoringPermissions({ role: 'operator', status: 'active' }), { readChannels: false, writeRules: false, writeDashboards: true });
  assert.deepEqual(monitoringPermissions({ role: 'admin', account_status: 'suspended' }), { readChannels: true, writeRules: false, writeDashboards: false });
  assert.deepEqual(monitoringPermissions({ role: 'admin', status: 'active' }), { readChannels: true, writeRules: true, writeDashboards: true });
  assert.equal(monitoringPermissions({ role: 'admin', status: 'invited' }).writeRules, false);
});
test('rule validation enforces numeric thresholds, hysteresis and whole second durations', () => {
  assert.equal(validateRule(rule(), 'number'), '');
  assert.match(validateRule(rule(), 'text'), /numeric$/);
  assert.match(validateRule(rule({ recovery: 81 }), 'number'), /recovery$/);
  assert.equal(validateRule(rule({ operator: 'lt', recovery: 85 }), 'number'), '');
  assert.match(validateRule(rule({ operator: 'lt', recovery: 70 }), 'number'), /recovery$/);
  assert.match(validateRule(rule({ for_seconds: -1 }), 'number'), /duration$/);
  assert.match(validateRule(rule({ cooldown_seconds: 1.5 }), 'number'), /duration$/);
  assert.match(validateRule(rule({ cooldown_seconds: 59 }), 'number'), /duration$/);
  assert.match(validateRule(rule({ for_seconds: 604801 }), 'number'), /duration$/);
  assert.match(validateRule(rule({ operator: '' }), 'number'), /threshold$/);
  assert.equal(validateRule(rule({ kind: 'offline', metric: '' })), '');
  assert.equal(validateRule(rule({ kind: 'no_data' }), 'text'), '');
  assert.match(validateRule(rule({ kind: 'no_data', missing_seconds: 0 }), 'text'), /duration$/);
});
test('channel payload preserves a saved secret by omission and never includes read-only fields', () => {
  const input = { name: ' Telegram ', kind: 'telegram', destination: ' -123 ', enabled: true, secret: '   ', has_secret: true, id: 42 };
  assert.deepEqual(channelPayload(input), { name: 'Telegram', kind: 'telegram', destination: '-123', enabled: true });
  assert.equal(validateChannel(input, true), '');
  assert.match(validateChannel(input, false), /secret$/);
  assert.equal(channelPayload({ ...input, secret: ' replacement ' }).secret, 'replacement');
  assert.equal('secret' in channelPayload({ ...input, kind: 'email', secret: 'do-not-send' }), false);
  assert.match(validateChannel({ ...input, destination: '@username', secret: 'token' }), /chat$/);
  assert.match(validateChannel({ ...input, kind: 'email', destination: 'bad' }), /email$/);
  assert.equal(validateChannel({ ...input, kind: 'email', destination: 'ops@example.test' }), '');
});
test('account guard blocks requests before dispatch after switch, logout or abort', async () => {
  let calls = 0;
  const request = async () => { calls++; return 'data'; };
  const controller = new AbortController();
  await assert.rejects(accountRequest(request, 'account-A', () => 'account-B', controller.signal), { name: 'AbortError' });
  await assert.rejects(accountRequest(request, '', () => '', controller.signal), { name: 'AbortError' });
  controller.abort();
  await assert.rejects(accountRequest(request, 'account-A', () => 'account-A', controller.signal), { name: 'AbortError' });
  assert.equal(calls, 0);
});
test('late successful and failed responses cannot leak across account switches', async () => {
  let current = 'account-A';
  let resolve;
  const pending = accountRequest(() => new Promise(done => { resolve = done; }), 'account-A', () => current, new AbortController().signal);
  current = 'account-B'; resolve({ secret: 'account-A data' });
  await assert.rejects(pending, { name: 'AbortError' });
  current = 'account-A';
  let reject;
  const failed = accountRequest(() => new Promise((_, fail) => { reject = fail; }), 'account-A', () => current, new AbortController().signal);
  current = 'account-B'; reject(new Error('account-A error'));
  await assert.rejects(failed, { name: 'AbortError' });
});
test('unmount abort rejects a late response while ordinary server errors remain readable', async () => {
  const controller = new AbortController();
  let resolve;
  const pending = accountRequest(() => new Promise(done => { resolve = done; }), 'A', () => 'A', controller.signal);
  controller.abort(); resolve('old');
  await assert.rejects(pending, { name: 'AbortError' });
  await assert.rejects(accountRequest(async () => { throw new Error('Version conflict'); }, 'A', () => 'A', new AbortController().signal), /Version conflict/);
  assert.equal(await accountRequest(async () => 'current', 'A', () => 'A', new AbortController().signal), 'current');
});
test('all Monitoring copy, documentation and validation errors have matching EN/RU keys', () => {
  const english = Object.keys(en).filter(key => key.startsWith('monitoring.') || key.startsWith('docs.') || key === 'nav.monitoring').sort();
  const russian = Object.keys(ru).filter(key => key.startsWith('monitoring.') || key.startsWith('docs.') || key === 'nav.monitoring').sort();
  assert.deepEqual(english, russian);
  const dynamic = [
    ...Object.values(WIDGET_TYPES).map(type => type.label),
    ...AGGREGATIONS.map(a => `monitoring.aggregation.${a}`), ...WINDOWS.map(w => `monitoring.window.${w}`),
    ...RULE_KINDS.map(k => `monitoring.ruleKind.${k}`), ...CHANNEL_KINDS.map(k => `monitoring.channelKind.${k}`),
  ];
  for (const key of dynamic) { assert.ok(en[key], key); assert.ok(ru[key], key); }
  const files = ['Shared.tsx', 'WidgetView.tsx', 'WidgetWizard.tsx', 'Dashboards.tsx', 'Alerts.tsx', 'Channels.tsx', 'History.tsx', '../../pages/MonitoringPage.tsx', '../../pages/DocsPage.tsx', '../../api/monitoring.ts'];
  for (const file of files) {
    const code = readFileSync(new URL(file, import.meta.url), 'utf8');
    for (const [, key] of code.matchAll(/['"]((?:monitoring|docs|common|nav)\.[A-Za-z0-9_.]+)['"]/g)) { assert.ok(en[key], `${file}: ${key}`); assert.ok(ru[key], `${file}: ${key}`); }
  }
  for (const key of english) assert.deepEqual([...en[key].matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort(), [...ru[key].matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort(), key);
});
