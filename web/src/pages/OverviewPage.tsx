import { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { SeverityBadge } from '../components/StatusBadge';
import type { Device, EventRow, Overview } from '../types';

export function OverviewPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [events, setEvents] = useState<EventRow[]>([]);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      setOverview(await api<Overview>('/overview'));
      setEvents(await api<EventRow[]>('/events?limit=15'));
      await api<Device[]>('/devices');
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status' || msg.event === 'telemetry' || msg.event === 'event') {
        void load();
      }
    });
  }, [load]);

  const o = overview || ({} as Overview);

  return (
    <div>
      <h1 className="h3 mb-3">Overview</h1>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="row g-3 mb-4">
        {[
          ['Online', o.online_devices || 0, 'var(--ok)'],
          ['Offline', o.offline_devices || 0, 'var(--crit)'],
          ['Total', o.total_devices || 0, undefined],
          ['Events 24h', o.recent_events_24h || 0, undefined],
          ['MQTT', o.mqtt_connected ? 'connected' : 'down', undefined],
        ].map(([label, value, color]) => (
          <div className="col-6 col-md-4 col-xl" key={String(label)}>
            <div className="stat-card">
              <div className="label">{label}</div>
              <div className="value" style={color ? { color: String(color), fontSize: typeof value === 'string' ? '1rem' : undefined } : undefined}>
                {value}
              </div>
            </div>
          </div>
        ))}
      </div>
      <div className="panel">
        <h2 className="h6 text-uppercase muted">Recent events</h2>
        <div className="table-responsive">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>Time</th>
                <th>Device</th>
                <th>Severity</th>
                <th>Code</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {events.length ? (
                events.map((e, i) => (
                  <tr key={i}>
                    <td className="mono">{new Date(e.ts).toLocaleString()}</td>
                    <td className="mono">
                      {e.product_id}.{e.device_id}
                    </td>
                    <td>
                      <SeverityBadge value={e.severity} />
                    </td>
                    <td className="mono">{e.code}</td>
                    <td>{e.message || ''}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={5} className="muted">
                    No events
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
