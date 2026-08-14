import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { SeverityBadge } from '../components/StatusBadge';
import { formatAge } from '../utils/format';
import type { MQTTStatus } from '../types';

export function MqttPage() {
  const [st, setSt] = useState<MQTTStatus | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      setSt(await api<MQTTStatus>('/mqtt'));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    const t = window.setInterval(() => void load(), 2500);
    const unsub = platformWS.subscribe((msg) => {
      if (msg.event === 'mqtt' || msg.event === 'device.status') void load();
    });
    return () => {
      window.clearInterval(t);
      unsub();
    };
  }, [load]);

  const s = st || ({ listening: false, client_count: 0, clients: [], events: [] } as MQTTStatus);

  return (
    <div>
      <h1 className="h3 mb-3">MQTT</h1>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="row g-3 mb-4">
        <div className="col-6 col-md-3">
          <div className="stat-card">
            <div className="label">Broker</div>
            <div className="value" style={{ fontSize: '1.1rem', color: s.listening ? 'var(--ok)' : 'var(--crit)' }}>
              {s.listening ? 'listening' : 'down'}
            </div>
          </div>
        </div>
        <div className="col-6 col-md-3">
          <div className="stat-card">
            <div className="label">Listen</div>
            <div className="value mono" style={{ fontSize: '1.1rem' }}>
              {s.addr || '—'}
            </div>
          </div>
        </div>
        <div className="col-6 col-md-3">
          <div className="stat-card">
            <div className="label">Sessions</div>
            <div className="value">{s.client_count}</div>
          </div>
        </div>
        {s.ws_addr ? (
          <div className="col-6 col-md-3">
            <div className="stat-card">
              <div className="label">MQTT WS</div>
              <div className="value mono" style={{ fontSize: '0.95rem' }}>
                {s.ws_addr}
              </div>
            </div>
          </div>
        ) : (
          <div className="col-6 col-md-3">
            <div className="stat-card">
              <div className="label">MQTT WS</div>
              <div className="value muted" style={{ fontSize: '1.1rem' }}>
                off
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="panel mb-4">
        <h2 className="h6 text-uppercase muted">Connected clients</h2>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Kind</th>
                <th>Username</th>
                <th>Client ID</th>
                <th>Remote</th>
                <th>Subscriptions</th>
                <th>Connected</th>
                <th>Last seen</th>
              </tr>
            </thead>
            <tbody>
              {s.clients.length ? (
                s.clients.map((c) => (
                  <tr key={c.client_id || c.username + c.remote}>
                    <td>
                      <span className={`badge ${c.kind === 'device' ? 'badge-online' : c.kind === 'nodered' ? 'badge-info' : 'badge-unknown'}`}>
                        {c.kind}
                      </span>
                    </td>
                    <td className="mono">
                      {c.device_pk ? <Link to={`/devices/${c.device_pk}`}>{c.username}</Link> : c.username || '—'}
                    </td>
                    <td className="mono small">{c.client_id}</td>
                    <td className="mono small">{c.remote || '—'}</td>
                    <td className="mono small">{(c.subscriptions || []).join(', ') || '—'}</td>
                    <td className="muted small">{formatAge(c.connected_at)}</td>
                    <td className="muted small">{formatAge(c.last_seen_at)}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={7} className="muted">
                    No MQTT clients connected
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Broker events</h2>
        <p className="muted small">In-memory trail (auth, sessions, ACL, ingest). Telemetry ingest is debug in stdout; it still appears here.</p>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Time</th>
                <th>Level</th>
                <th>Kind</th>
                <th>User</th>
                <th>Topic</th>
                <th>Message</th>
                <th>Bytes</th>
              </tr>
            </thead>
            <tbody>
              {s.events.length ? (
                s.events.map((e, i) => (
                  <tr key={`${e.ts}-${e.kind}-${i}`}>
                    <td className="mono small">{new Date(e.ts).toLocaleString()}</td>
                    <td>
                      <SeverityBadge value={e.level === 'warn' ? 'warning' : e.level === 'error' ? 'error' : 'info'} />
                    </td>
                    <td className="mono">{e.kind}</td>
                    <td className="mono small">{e.username || e.client_id || '—'}</td>
                    <td className="mono small">{e.topic || '—'}</td>
                    <td className="small">{e.message || ''}</td>
                    <td className="mono small">{e.bytes ? e.bytes : ''}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={7} className="muted">
                    No MQTT events yet — connect a device or Node-RED
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
