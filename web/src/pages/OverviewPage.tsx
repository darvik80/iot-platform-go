import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { useAuth } from '../auth/AuthContext';
import { SeverityBadge } from '../components/StatusBadge';
import { useT } from '../i18n/I18nContext';
import type { Device, EventRow, MQTTStatus, Overview } from '../types';

export function OverviewPage() {
  const t = useT();
  const { settings } = useAuth();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [events, setEvents] = useState<EventRow[]>([]);
  const [mqtt, setMqtt] = useState<MQTTStatus | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [ov, ev, mq] = await Promise.all([
        api<Overview>('/overview'),
        api<EventRow[]>('/events?limit=15'),
        api<MQTTStatus>('/mqtt'),
      ]);
      setOverview(ov);
      setEvents(ev);
      setMqtt(mq);
      await api<Device[]>('/devices');
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status' || msg.event === 'telemetry' || msg.event === 'event' || msg.event === 'mqtt') {
        void load();
      }
    });
  }, [load]);

  const o = overview || ({} as Overview);

  return (
    <div>
      <h1 className="h3 mb-3">{t('overview.title')}</h1>
      {error && <div className="alert alert-danger py-2">{String(error)}</div>}
      <div className="row g-3 mb-4">
        {[
          [t('overview.online'), o.online_devices || 0, 'var(--ok)'],
          [t('overview.offline'), o.offline_devices || 0, 'var(--crit)'],
          [t('overview.total'), o.total_devices || 0, undefined],
          [t('overview.events24h'), o.recent_events_24h || 0, undefined],
          [
            'MQTT',
            o.mqtt_connected ? t('overview.clients', { count: o.mqtt_clients ?? mqtt?.client_count ?? 0 }) : t('common.down'),
            o.mqtt_connected ? 'var(--ok)' : 'var(--crit)',
          ],
        ].map(([label, value, color]) => (
          <div className="col-6 col-md-4 col-xl" key={String(label)}>
            <div className="stat-card">
              <div className="label">{label}</div>
              <div className="value" style={color ? { color: String(color), fontSize: typeof value === 'string' ? '1rem' : undefined } : undefined}>
                {String(value)}
              </div>
            </div>
          </div>
        ))}
      </div>
      <div className="panel mb-4">
        <div className="d-flex align-items-center justify-content-between gap-2 mb-2">
          <h2 className="h6 text-uppercase muted mb-0">{t('overview.mqttSessions')}</h2>
          <Link className="small" to="/mqtt">
            {t('overview.openMqtt')}
          </Link>
        </div>
        <div className="table-responsive">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t('overview.kind')}</th>
                <th>{t('overview.user')}</th>
                <th>{t('overview.remote')}</th>
                <th>{t('overview.subscriptions')}</th>
              </tr>
            </thead>
            <tbody>
              {(mqtt?.clients || []).length ? (
                mqtt!.clients.map((c) => (
                  <tr key={c.client_id || c.username + c.remote}>
                    <td className="mono small">{c.kind}</td>
                    <td className="mono">
                      {c.device_pk ? <Link to={`/devices/${c.device_pk}`}>{c.username}</Link> : c.username || '—'}
                    </td>
                    <td className="mono small">{c.remote || '—'}</td>
                    <td className="mono small">{(c.subscriptions || []).join(', ') || '—'}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={4} className="muted">
                    No MQTT clients — {o.mqtt_connected ? t('overview.brokerUp') : t('overview.brokerDown')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
      <div className="panel">
        <h2 className="h6 text-uppercase muted">{t('overview.recentEvents')}</h2>
        <div className="table-responsive">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t('common.time')}</th>
                <th>{t('common.device')}</th>
                <th>{t('overview.severity')}</th>
                <th>{t('overview.code')}</th>
                <th>{t('common.message')}</th>
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
                    {t('overview.noEvents')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="mt-4 d-flex flex-wrap gap-3 small">
        <Link to="/docs" className="muted">
          📖 {t('overview.documentation')}
        </Link>
        <Link to="/docs#getting-started" className="muted">
          🚀 {t('overview.gettingStarted')}
        </Link>
        <Link to="/docs#mqtt-connection" className="muted">
          🔌 {t('overview.mqttConnection')}
        </Link>
        <Link to="/docs#topic-reference" className="muted">
          📋 {t('overview.topicReference')}
        </Link>
        <Link to="/docs#automation" className="muted">
          🤖 {t('overview.automation')}
        </Link>
      </div>

      {(settings?.version || settings?.commit) && (
        <div className="mt-3 text-muted small">
          {settings?.version && <span>v{settings.version}</span>}
          {settings?.commit && settings?.commit !== 'unknown' && (
            <span className="ms-2 mono">{settings.commit}</span>
          )}
        </div>
      )}
    </div>
  );
}
