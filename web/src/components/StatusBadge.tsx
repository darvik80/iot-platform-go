export function StatusBadge({ status }: { status?: string }) {
  const s = status || 'unknown';
  const cls =
    s === 'online' ? 'badge-online' : s === 'offline' ? 'badge-offline' : 'badge-unknown';
  return <span className={`badge ${cls}`}>{s}</span>;
}

export function SeverityBadge({ value }: { value: string }) {
  const cls =
    value === 'critical' || value === 'ERROR'
      ? 'badge-critical'
      : value === 'warning' || value === 'WARN'
        ? 'badge-warning'
        : 'badge-info';
  return <span className={`badge ${cls}`}>{value}</span>;
}
