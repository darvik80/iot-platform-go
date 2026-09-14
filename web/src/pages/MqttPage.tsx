import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { SeverityBadge } from '../components/StatusBadge';
import { formatAge } from '../utils/format';
import { useT } from '../i18n/I18nContext';
import type { MQTTStatus } from '../types';

export function MqttPage() {
  const t = useT();
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
      <h1 className="h3 mb-3">{t('mqtt.title')}</h1>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="row g-3 mb-4">
        <div className="col-6 col-md-3">
          <div className="stat-card">
            <div className="label">{t('common.status')}</div>
            <div className="value" style={{ fontSize: '1.1rem', color: s.listening ? 'var(--ok)' : 'var(--crit)' }}>
              {s.listening ? t('mqtt.listening') : t('common.down')}
            </div>
          </div>
        </div>
        <div className="col-6 col-md-3">
          <div className="stat-card">
            <div className="label">{t('mqtt.sessions')}</div>
            <div className="value">{s.client_count}</div>
          </div>
        </div>
      </div>
      <div className="row g-3 mb-4">
        <div className="col-12 col-md-4">
          <div className="stat-card">
            <div className="label">TCP</div>
            <div className="value mono" style={{ fontSize: '0.9rem' }}>
              {s.addr || '\u2014'}
            </div>
          </div>
        </div>
        <div className="col-12 col-md-4">
          <div className="stat-card">
            <div className="label">WS</div>
            <div className="value mono" style={{ fontSize: '0.9rem' }}>
              {s.ws_addr || <span className="muted">off</span>}
            </div>
          </div>
        </div>
        <div className="col-12 col-md-4">
          <div className="stat-card">
            <div className="label">TLS</div>
            <div className="value mono" style={{ fontSize: '0.9rem' }}>
              {s.tls_addr || <span className="muted">off</span>}
            </div>
            {s.tls_addr && (
              <a className="small" href="/api/v1/mqtt/ca.pem" download>
                {t('mqtt.downloadCa')}
              </a>
            )}
          </div>
        </div>
      </div>

      <div className="panel mb-4">
        <h2 className="h6 text-uppercase muted">{t('mqtt.connectedClients')}</h2>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>{t('overview.kind')}</th>
                <th>{t('mqtt.username')}</th>
                <th>{t('mqtt.clientId')}</th>
                <th>{t('overview.remote')}</th>
                <th>{t('overview.subscriptions')}</th>
                <th>{t('mqtt.connected')}</th>
                <th>{t('mqtt.lastSeen')}</th>
              </tr>
            </thead>
            <tbody>
              {s.clients.length ? (
                s.clients.map((c) => (
                  <tr key={c.client_id || c.username + c.remote}>
                    <td>
                      <span className={`badge ${c.kind === 'device' ? 'badge-online' : c.kind === 'plugin' ? 'badge-info' : 'badge-unknown'}`}>
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
                    {t('mqtt.noClients')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">{t('mqtt.brokerEvents')}</h2>
        <p className="muted small">{t('mqtt.eventsDescription')}</p>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>{t('common.time')}</th>
                <th>{t('mqtt.level')}</th>
                <th>{t('overview.kind')}</th>
                <th>{t('overview.user')}</th>
                <th>{t('mqtt.topic')}</th>
                <th>{t('common.message')}</th>
                <th>{t('mqtt.bytes')}</th>
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
                    {t('mqtt.noEvents')}
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
