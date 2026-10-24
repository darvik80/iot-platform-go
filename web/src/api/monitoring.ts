// This registry is the source of truth for wizard choices, validation and rendering.
export const WIDGET_TYPES = {
  line: { label: 'monitoring.type.line', numericOnly: true, view: 'chart', bars: false },
  bar: { label: 'monitoring.type.bar', numericOnly: true, view: 'chart', bars: true },
  stat: { label: 'monitoring.type.stat', numericOnly: false, view: 'stat', bars: false },
  gauge: { label: 'monitoring.type.gauge', numericOnly: true, view: 'gauge', bars: false },
  table: { label: 'monitoring.type.table', numericOnly: false, view: 'table', bars: false },
} as const;
export type WidgetType = keyof typeof WIDGET_TYPES;
export const AGGREGATIONS = ['avg', 'min', 'max', 'sum', 'last'] as const;
export const WINDOWS = [300, 900, 3600, 21600, 86400, 604800] as const;
export const COLORS = ['#3d9cf0', '#3ecf8e', '#e6a23c', '#f07178', '#b69cff', '#46c9d4', '#ed91c1', '#b9c96c'];
export const MAX_SOURCES = 8;
export const MAX_WIDGETS = 24;
export const RULE_KINDS = ['threshold', 'no_data', 'offline'] as const;
export const CHANNEL_KINDS = ['email', 'telegram'] as const;
export interface Metric { key: string; kind: 'number' | 'boolean' | 'text'; last_seen: number }
export interface WidgetSource { device_id: number; key: string; label: string; color: string }
export interface Widget {
  id: string;
  title: string;
  type: WidgetType;
  sources: WidgetSource[];
  window_seconds: number;
  aggregation: typeof AGGREGATIONS[number];
  width: 6 | 12;
  min: number;
  max: number;
}
export interface MonitoringDashboard { id: number; account_id: number; name: string; version: number; widgets: Widget[] }
export interface Point { ts: number; value: number | string | boolean | null }
export interface Series extends WidgetSource { points: Point[] }
export interface QueryResult { from: number; to: number; series: Series[] }
export interface AlertRule {
  id: number; account_id: number; name: string; device_id: number; metric: string;
  kind: typeof RULE_KINDS[number]; operator: 'gt' | 'lt'; threshold: number; recovery: number;
  for_seconds: number; missing_seconds: number; cooldown_seconds: number; enabled: boolean;
  channel_ids: number[]; state: string; since: number;
}
export type RuleInput = Omit<AlertRule, 'id' | 'account_id' | 'state' | 'since'>;
export interface Channel {
  id: number; account_id: number; name: string; kind: typeof CHANNEL_KINDS[number];
  destination: string; enabled: boolean; has_secret: boolean;
}
export interface ChannelInput { name: string; kind: Channel['kind']; destination: string; enabled: boolean; secret?: string }
export interface AlertEvent { id: number; rule_id: number; name: string; state: string; message: string; created_at: number }
export interface Delivery { id: number; event_id: number; channel_id: number; status: string; attempts: number; last_error: string; next_attempt: number }

export function monitoringPermissions(membership: { role: string; account_status?: string; status?: string } | null) {
  const admin = membership?.role === 'admin';
  const writable = !!membership && membership.account_status !== 'suspended' && (!membership.status || membership.status === 'active');
  return { readChannels: admin, writeRules: admin && writable, writeDashboards: writable && (admin || membership?.role === 'operator') };
}
export function sourceId(source: Pick<WidgetSource, 'device_id' | 'key'>) {
  return JSON.stringify([source.device_id, source.key]);
}
export function newWidgetID(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');
}
export function createWidget(id: string): Widget {
  return { id, title: '', type: 'line', sources: [], window_seconds: 3600, aggregation: 'avg', width: 6, min: 0, max: 100 };
}
export function acceptsMetric(type: WidgetType, aggregation: Widget['aggregation'], kind: Metric['kind']) {
  return kind === 'number' || (!WIDGET_TYPES[type].numericOnly && aggregation === 'last');
}
export function validateSources(sources: WidgetSource[]) {
  if (!sources.length || sources.length > MAX_SOURCES) return 'monitoring.validation.sources';
  if (new Set(sources.map(sourceId)).size !== sources.length) return 'monitoring.validation.duplicate';
  if (sources.some(s => !Number.isSafeInteger(s.device_id) || s.device_id <= 0 || !s.key)) return 'monitoring.validation.sources';
  return '';
}
export function validateWidget(widget: Widget, kinds?: Record<string, Metric['kind']>) {
  const sourcesError = validateSources(widget.sources);
  if (sourcesError) return sourcesError;
  if (!widget.title.trim()) return 'monitoring.validation.title';
  if (!WIDGET_TYPES[widget.type] || !AGGREGATIONS.includes(widget.aggregation)) return 'monitoring.validation.type';
  if (!Number.isSafeInteger(widget.window_seconds) || widget.window_seconds < 60 || widget.window_seconds > 31 * 86400) return 'monitoring.validation.window';
  if (widget.width !== 6 && widget.width !== 12) return 'monitoring.validation.width';
  if (!Number.isFinite(widget.min) || !Number.isFinite(widget.max) || (WIDGET_TYPES[widget.type].view === 'gauge' && widget.min >= widget.max)) return 'monitoring.validation.range';
  if (widget.sources.some(s => !/^#[0-9a-f]{6}$/i.test(s.color))) return 'monitoring.validation.color';
  if (kinds && widget.sources.some(s => !kinds[sourceId(s)] || !acceptsMetric(widget.type, widget.aggregation, kinds[sourceId(s)]))) return 'monitoring.validation.numeric';
  return '';
}
export function moveWidget(widgets: Widget[], index: number, direction: -1 | 1) {
  const next = [...widgets];
  const target = index + direction;
  if (index >= 0 && index < next.length && target >= 0 && target < next.length) [next[index], next[target]] = [next[target], next[index]];
  return next;
}
export function validateRule(rule: RuleInput, kind?: Metric['kind']) {
  if (!rule.name.trim()) return 'monitoring.validation.title';
  if (!Number.isSafeInteger(rule.device_id) || rule.device_id <= 0) return 'monitoring.validation.device';
  if (rule.kind !== 'offline' && !rule.metric) return 'monitoring.validation.metric';
  if (rule.kind === 'threshold') {
    if (kind !== 'number') return 'monitoring.validation.numeric';
    if (!Number.isFinite(rule.threshold) || !Number.isFinite(rule.recovery) || !['gt', 'lt'].includes(rule.operator)) return 'monitoring.validation.threshold';
    if ((rule.operator === 'gt' && rule.recovery > rule.threshold) || (rule.operator === 'lt' && rule.recovery < rule.threshold)) return 'monitoring.validation.recovery';
  }
  if ([rule.for_seconds, rule.cooldown_seconds, rule.missing_seconds].some(n => !Number.isSafeInteger(n) || n < 0 || n > 604800) || rule.missing_seconds < 1 || rule.cooldown_seconds < 60) return 'monitoring.validation.duration';
  return '';
}
export function validateChannel(channel: ChannelInput, hasSecret = false) {
  if (!channel.name.trim()) return 'monitoring.validation.title';
  if (channel.kind === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(channel.destination.trim())) return 'monitoring.validation.email';
  if (channel.kind === 'telegram' && !/^-?\d+$/.test(channel.destination.trim())) return 'monitoring.validation.chat';
  if (channel.kind === 'telegram' && !hasSecret && !channel.secret?.trim()) return 'monitoring.validation.secret';
  return '';
}
export function channelPayload(channel: ChannelInput): ChannelInput {
  return { name: channel.name.trim(), kind: channel.kind, destination: channel.destination.trim(), enabled: channel.enabled,
    ...(channel.kind === 'telegram' && channel.secret?.trim() ? { secret: channel.secret.trim() } : {}) };
}
export function numeric(value: Point['value'] | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}
export function latestPoint(points: Point[]): Point | undefined {
  return points.reduce<Point | undefined>((latest, point) => !latest || point.ts >= latest.ts ? point : latest, undefined);
}
export function formatValue(value: Point['value'] | undefined, locale: string, noData: string, yes: string, no: string) {
  if (value == null || (typeof value === 'number' && !Number.isFinite(value))) return noData;
  if (typeof value === 'boolean') return value ? yes : no;
  if (typeof value === 'number') return new Intl.NumberFormat(locale, { maximumFractionDigits: 3 }).format(value);
  return value;
}
export function gaugePercent(value: Point['value'] | undefined, min: number, max: number): number | null {
  if (!numeric(value) || !Number.isFinite(min) || !Number.isFinite(max) || min >= max) return null;
  return Math.max(0, Math.min(100, (value - min) / (max - min) * 100));
}
export function chartDomain(result: QueryResult, includeZero = false) {
  const values = result.series.flatMap(s => s.points.filter(p => numeric(p.value)).map(p => p.value as number));
  if (!values.length) return null;
  let min = Math.min(...values, ...(includeZero ? [0] : []));
  let max = Math.max(...values, ...(includeZero ? [0] : []));
  if (min === max) { const pad = Math.max(Math.abs(min) * 0.05, 1); min -= pad; max += pad; }
  return { min, max, from: result.from, to: Math.max(result.to, result.from + 1) };
}
// Explicit null/text samples break the line instead of fabricating zeros or bridging gaps.
export function lineSegments(points: Point[]) {
  const segments: Point[][] = [];
  let segment: Point[] = [];
  for (const point of [...points].sort((a, b) => a.ts - b.ts)) {
    if (numeric(point.value)) segment.push(point);
    else if (segment.length) { segments.push(segment); segment = []; }
  }
  if (segment.length) segments.push(segment);
  return segments;
}

// Capture the account's token at mount, not after an asynchronous operation. Also check
// after completion: a response arriving during an account switch must never be applied.
export async function accountRequest<T>(request: () => Promise<T>, token: string, currentToken: () => string, signal: AbortSignal): Promise<T> {
  const check = () => { if (signal.aborted || !token || currentToken() !== token) throw new DOMException('Account request cancelled', 'AbortError'); };
  check();
  try {
    const result = await request();
    check();
    return result;
  } catch (error) {
    check();
    throw error;
  }
}
