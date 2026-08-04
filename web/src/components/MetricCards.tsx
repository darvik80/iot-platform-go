import type { Product, ProductMetric, TelemetrySample } from '../types';
import { formatAge, formatBytes, formatDuration, humanizeKey, sampleValue } from '../utils/format';

const GROUP_ORDER = ['System', 'Network', 'Memory', 'Storage', 'Sensors', 'Other'];

function metricFormatter(m: ProductMetric) {
  const fmt = m.format || '';
  const unit = m.unit || '';
  const prec = m.precision;
  return (v: unknown) => {
    switch (fmt) {
      case 'bytes':
        return formatBytes(v);
      case 'duration':
        return formatDuration(v);
      case 'percent':
        return Number(v).toFixed(prec != null ? prec : 0) + (unit || '%');
      case 'rssi':
        return Number(v) + ' dBm';
      case 'bool':
        return v === true || v === 1 || v === 'true' ? 'yes' : 'no';
      case 'text':
        return String(v);
      case 'number':
        return (prec != null ? Number(v).toFixed(prec) : String(v)) + (unit ? ' ' + unit : '');
      default:
        if (unit === 'bytes' || /bytes|heap|psram/i.test(m.key)) return formatBytes(v);
        if (typeof v === 'number' && unit)
          return Number(v).toFixed(prec != null ? prec : Number.isInteger(v) ? 0 : 2) + ' ' + unit;
        return String(v);
    }
  };
}

export function MetricCards({
  latest,
  product,
}: {
  latest?: Record<string, TelemetrySample>;
  product?: Product;
}) {
  const metaMap: Record<string, { label: string; group: string; description?: string; format?: (v: unknown) => string }> = {};
  (product?.metrics || []).forEach((m) => {
    if (!m?.key) return;
    metaMap[m.key] = {
      label: m.label || humanizeKey(m.key),
      group: m.group || 'Other',
      description: m.description,
      format: metricFormatter(m),
    };
  });

  const entries = Object.entries(latest || {});
  if (!entries.length) return <p className="muted mb-0">No telemetry yet</p>;

  let newest = 0;
  const groups: Record<string, { key: string; label: string; value: string; description?: string }[]> = {};
  for (const [key, s] of entries) {
    const raw = sampleValue(s);
    const meta = metaMap[key] || { label: humanizeKey(key), group: 'Other' };
    const group = meta.group || 'Other';
    if (!groups[group]) groups[group] = [];
    let value: string;
    try {
      value = meta.format ? meta.format(raw) : String(raw);
    } catch {
      value = String(raw);
    }
    groups[group].push({ key, label: meta.label, value, description: meta.description });
    if (s.ts > newest) newest = s.ts;
  }

  return (
    <>
      <div className="small muted mb-2">Last sample: {formatAge(newest)}{newest ? ` · ${new Date(newest).toLocaleString()}` : ''}</div>
      {GROUP_ORDER.filter((g) => groups[g]?.length).map((g) => (
        <div key={g} className="mb-3">
          <h6 className="text-uppercase muted small">{g}</h6>
          <div className="metrics">
            {groups[g].map((m) => (
              <div key={m.key} className="metric" title={m.description || m.key}>
                <div className="m-label">{m.label}</div>
                <div className="m-value">{m.value}</div>
                {m.label !== m.key ? <div className="m-key">{m.key}</div> : null}
              </div>
            ))}
          </div>
        </div>
      ))}
    </>
  );
}
