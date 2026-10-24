import { useId, useState } from 'react';
import { chartDomain, formatValue, gaugePercent, latestPoint, lineSegments, numeric, sourceId, WIDGET_TYPES, type Point, type QueryResult, type Widget } from '../../api/monitoring';
import { useLang, useT } from '../../i18n/I18nContext';
import { type MonitoringRequest, useMonitoringResource } from './hooks';
import { ResourceState } from './Shared';

export function WidgetQuery({ widget, request, refresh = 0, preview = false }: { widget: Widget; request: MonitoringRequest; refresh?: number; preview?: boolean }) {
  const query = useMonitoringResource<QueryResult>(request, '/monitoring/query', refresh, !preview, JSON.stringify({ widget }));
  return <>
    <ResourceState loading={query.loading} error={query.error} retry={query.reload} />
    {query.data && <WidgetView widget={widget} result={query.data} />}
  </>;
}
export function WidgetView({ widget, result }: { widget: Widget; result: QueryResult }) {
  const t = useT();
  const { lang } = useLang();
  const type = WIDGET_TYPES[widget.type];
  const data: QueryResult = { ...result, series: widget.sources.map(source => ({ ...source, points: result.series?.find(s => sourceId(s) === sourceId(source))?.points || [] })) };
  const value = (v: Point['value'] | undefined) => formatValue(v, lang, t('common.noData'), t('common.yes'), t('common.no'));
  const time = (ts: number) => new Date(ts).toLocaleString(lang);
  const table = <div className="table-responsive monitoring-data-table"><table className="table table-sm mb-0">
    <caption>{widget.title}</caption><thead><tr><th>{t('monitoring.source')}</th><th>{t('common.time')}</th><th>{t('monitoring.value')}</th></tr></thead>
    <tbody>{data.series.flatMap(s => [...s.points].sort((a, b) => b.ts - a.ts).map((p, i) => <tr key={`${sourceId(s)}-${i}`}><td>{s.label || s.key}</td><td>{time(p.ts)}</td><td className="mono">{value(p.value)}</td></tr>))}</tbody>
  </table></div>;
  if (!data.series.some(s => s.points.length)) return <p className="muted py-4 text-center">{t('monitoring.noSamples')}</p>;
  return <>
    {type.view === 'chart' && <TimeChart widget={widget} result={data} />}
    {type.view === 'stat' && <div className="metrics">{data.series.map(s => { const point = latestPoint(s.points); return <div className="metric" key={sourceId(s)}>
      <div className="small" style={{ color: s.color }}>{s.label || s.key}</div><div className="fs-3 mono text-break">{value(point?.value)}</div>
      {point && <div className="small muted">{time(point.ts)}</div>}
    </div>; })}</div>}
    {type.view === 'gauge' && <div className="d-flex flex-column gap-3">{data.series.map(s => {
      const point = latestPoint(s.points);
      const percent = gaugePercent(point?.value, widget.min, widget.max);
      return <div key={sourceId(s)}>
        <div className="d-flex justify-content-between gap-2 mb-2"><span>{s.label || s.key}</span><strong className="mono">{value(point?.value)}</strong></div>
        {percent !== null ? <div className="progress" role="meter" aria-label={s.label || s.key} aria-valuemin={widget.min} aria-valuemax={widget.max} aria-valuenow={Math.max(widget.min, Math.min(widget.max, point!.value as number))} aria-valuetext={value(point?.value)}><div className="progress-bar" style={{ width: `${percent}%`, background: s.color }} /></div> : <p className="muted small">{t('common.noData')}</p>}
        <div className="d-flex justify-content-between small muted mt-1"><span>{value(widget.min)}</span><span>{value(widget.max)}</span></div>
      </div>;
    })}</div>}
    {type.view === 'table' ? table : <details className="mt-3 small"><summary className="muted">{t('monitoring.dataTable')}</summary>{table}</details>}
    <div className="small muted mt-2">{t(`monitoring.aggregation.${widget.aggregation}`)} · {time(result.from)} – {time(result.to)}</div>
  </>;
}
function TimeChart({ widget, result }: { widget: Widget; result: QueryResult }) {
  const t = useT();
  const { lang } = useLang();
  const id = useId();
  const [hover, setHover] = useState<number | null>(null);
  const bars = WIDGET_TYPES[widget.type].bars;
  const domain = chartDomain(result, bars);
  if (!domain) return <p className="muted">{t('monitoring.noNumericSamples')}</p>;
  const W = 640, H = 250, left = 64, right = 20, top = 18, bottom = 42;
  const x = (ts: number) => left + (ts - domain.from) / (domain.to - domain.from) * (W - left - right);
  const y = (v: number) => top + (domain.max - v) / (domain.max - domain.min) * (H - top - bottom);
  const timestamps = [...new Set(result.series.flatMap(s => s.points.filter(p => numeric(p.value)).map(p => p.ts)))].sort((a, b) => a - b);
  const selectedTime = hover == null ? null : timestamps[Math.min(hover, timestamps.length - 1)];
  const fmt = (v: Point['value']) => formatValue(v, lang, t('common.noData'), t('common.yes'), t('common.no'));
  const time = (ts: number) => new Date(ts).toLocaleString(lang, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  const barWidth = Math.max(0.5, Math.min(22, (W - left - right) / Math.max(timestamps.length, 1) / Math.max(result.series.length, 1) * 0.8));
  const nearest = (ts: number) => timestamps.reduce((best, candidate, i) => Math.abs(candidate - ts) < Math.abs(timestamps[best] - ts) ? i : best, 0);
  return <div>
    <svg className="monitoring-chart" viewBox={`0 0 ${W} ${H}`} role="img" aria-labelledby={`${id}-title ${id}-help`} tabIndex={0}
      onMouseMove={e => { const rect = e.currentTarget.getBoundingClientRect(); const position = (e.clientX - rect.left) / rect.width * W; setHover(nearest(domain.from + (position - left) / (W - left - right) * (domain.to - domain.from))); }}
      onMouseLeave={() => setHover(null)} onFocus={() => setHover(timestamps.length - 1)} onBlur={() => setHover(null)}
      onKeyDown={e => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') { e.preventDefault(); setHover(current => Math.max(0, Math.min(timestamps.length - 1, (current ?? timestamps.length - 1) + (e.key === 'ArrowLeft' ? -1 : 1)))); } }}>
      <title id={`${id}-title`}>{widget.title}</title><desc id={`${id}-help`}>{t('monitoring.chartHelp')}</desc>
      <defs><clipPath id={`${id}-clip`}><rect x={left} y={top} width={W - left - right} height={H - top - bottom} /></clipPath></defs>
      {[0, 1, 2, 3, 4].map(i => { const v = domain.min + (domain.max - domain.min) * i / 4; return <g key={i}>
        <line x1={left} x2={W - right} y1={y(v)} y2={y(v)} stroke="var(--border)" />
        <text x={left - 8} y={y(v) + 4} textAnchor="end" fill="var(--muted)" fontSize="11">{new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 2 }).format(v)}</text>
      </g>; })}
      {[0, 0.5, 1].map(fraction => { const ts = domain.from + (domain.to - domain.from) * fraction; return <text key={fraction} x={x(ts)} y={H - 15} textAnchor={fraction === 0 ? 'start' : fraction === 1 ? 'end' : 'middle'} fill="var(--muted)" fontSize="10">{time(ts)}</text>; })}
      <g clipPath={`url(#${id}-clip)`}>{result.series.map((series, si) => <g key={sourceId(series)}>
        {!bars && lineSegments(series.points).map((segment, i) => <polyline key={i} points={segment.map(p => `${x(p.ts)},${y(p.value as number)}`).join(' ')} fill="none" stroke={series.color} strokeWidth="2" />)}
        {series.points.filter(p => numeric(p.value)).map((p, i) => bars
          ? <rect key={i} x={x(p.ts) + (si - result.series.length / 2) * barWidth} y={Math.min(y(0), y(p.value as number))} width={barWidth} height={Math.max(1, Math.abs(y(0) - y(p.value as number)))} fill={series.color}><title>{`${series.label || series.key}: ${fmt(p.value)} · ${time(p.ts)}`}</title></rect>
          : <circle key={i} cx={x(p.ts)} cy={y(p.value as number)} r={series.points.length === 1 ? 4 : 2} fill={series.color}><title>{`${series.label || series.key}: ${fmt(p.value)} · ${time(p.ts)}`}</title></circle>)}
      </g>)}{selectedTime != null && <line x1={x(selectedTime)} x2={x(selectedTime)} y1={top} y2={H - bottom} stroke="var(--muted)" strokeDasharray="4 3" />}</g>
    </svg>
    <div className="d-flex flex-wrap gap-3 small" aria-live="polite">
      {selectedTime != null && <span className="muted">{time(selectedTime)}</span>}
      {result.series.map(s => <span key={sourceId(s)}><span className="monitoring-swatch" style={{ background: s.color }} />{s.label || s.key}{selectedTime != null && <>: <strong>{fmt(s.points.find(p => p.ts === selectedTime)?.value ?? null)}</strong></>}</span>)}
    </div>
  </div>;
}
