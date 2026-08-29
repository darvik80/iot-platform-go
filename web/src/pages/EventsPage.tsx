import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { SeverityBadge } from '../components/StatusBadge';
import type { Device, EventRow } from '../types';

export function EventsPage() {
  const [events, setEvents] = useState<EventRow[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [severity, setSeverity] = useState('');
  const [devicePK, setDevicePK] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const q = new URLSearchParams({ limit: '200' });
      if (severity) q.set('severity', severity);
      if (devicePK) q.set('device_id', devicePK);
      const [list, devs] = await Promise.all([api<EventRow[]>(`/events?${q}`), api<Device[]>('/devices')]);
      setEvents(list || []);
      setDevices(devs || []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [severity, devicePK]);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'event' || msg.event === 'device.status') void load();
    });
  }, [load]);

  return (
    <div>
      <h1 className="h3 mb-3">Events</h1>
      <p className="muted small mb-3">
        Discrete incidents: device online/offline, firmware <span className="code">v1/e</span> posts, alerts. Not telemetry
        (metrics) and not log lines.
      </p>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="d-flex flex-wrap gap-2 mb-3">
        <select className="form-select form-select-sm" style={{ maxWidth: 220 }} value={devicePK} onChange={(e) => setDevicePK(e.target.value)}>
          <option value="">All devices</option>
          {devices.map((d) => (
            <option key={d.id} value={d.id}>
              {d.product_id}.{d.device_id}
            </option>
          ))}
        </select>
        <select className="form-select form-select-sm" style={{ maxWidth: 160 }} value={severity} onChange={(e) => setSeverity(e.target.value)}>
          <option value="">All severities</option>
          {['info', 'warning', 'error', 'critical'].map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </div>
      <div className="panel table-responsive">
        <table className="table table-sm align-middle">
          <thead>
            <tr>
              <th>Time</th>
              <th>Device</th>
              <th>Type</th>
              <th>Severity</th>
              <th>Code</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={e.id || `${e.ts}-${e.product_id}-${e.device_id}-${e.code}`}>
                <td className="mono small">{new Date(e.ts).toLocaleString()}</td>
                <td className="mono">
                  {e.device_pk ? (
                    <Link to={`/devices/${e.device_pk}`}>
                      {e.product_id}.{e.device_id}
                    </Link>
                  ) : (
                    `${e.product_id}.${e.device_id}`
                  )}
                </td>
                <td className="mono small">{e.type || '—'}</td>
                <td>
                  <SeverityBadge value={e.severity} />
                </td>
                <td className="mono">{e.code}</td>
                <td>{e.message || ''}</td>
              </tr>
            ))}
            {!events.length && (
              <tr>
                <td colSpan={6} className="muted">
                  No events yet. Online/offline will appear when a device connects. Firmware can also publish{' '}
                  <span className="code">v1/e</span>:{' '}
                  <span className="code">{`{"type":"alert","severity":"warning","code":"low_battery","message":"..."}`}</span>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
