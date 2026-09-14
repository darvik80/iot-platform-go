import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { SeverityBadge } from '../components/StatusBadge';
import { useT } from '../i18n/I18nContext';
import type { Device, LogRow } from '../types';

export function LogsPage() {
  const t = useT();
  const [logs, setLogs] = useState<LogRow[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [devicePK, setDevicePK] = useState('');
  const [level, setLevel] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const q = new URLSearchParams({ limit: '200' });
      if (level) q.set('level', level);
      if (devicePK) q.set('device_id', devicePK);
      const [list, devs] = await Promise.all([api<LogRow[]>(`/logs?${q}`), api<Device[]>('/devices')]);
      setLogs(list || []);
      setDevices(devs || []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [devicePK, level]);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'log' || msg.event === 'device.status') void load();
    });
  }, [load]);

  return (
    <div>
      <h1 className="h3 mb-3">{t('logs.title')}</h1>
      <p className="muted small mb-3">
        {t('logs.description')}
      </p>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="d-flex flex-wrap gap-2 mb-3">
        <select className="form-select form-select-sm" style={{ maxWidth: 220 }} value={devicePK} onChange={(e) => setDevicePK(e.target.value)}>
          <option value="">{t('logs.allDevices')}</option>
          {devices.map((d) => (
            <option key={d.id} value={d.id}>
              {d.product_id}.{d.device_id}
            </option>
          ))}
        </select>
        <select className="form-select form-select-sm" style={{ maxWidth: 160 }} value={level} onChange={(e) => setLevel(e.target.value)}>
          <option value="">{t('logs.allLevels')}</option>
          {['debug', 'info', 'warn', 'error'].map((s) => (
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
              <th>{t('common.time')}</th>
              <th>{t('logs.level')}</th>
              <th>{t('common.device')}</th>
              <th>{t('logs.module')}</th>
              <th>{t('common.message')}</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((l, i) => (
              <tr key={i}>
                <td className="mono small">{new Date(l.ts).toLocaleString()}</td>
                <td><SeverityBadge value={l.level} /></td>
                <td className="mono">
                  <Link to={`/devices/${l.product_id}.${l.device_id}`} className="text-decoration-none">
                    {l.product_id}.{l.device_id}
                  </Link>
                </td>
                <td>{l.module}</td>
                <td>{l.message}</td>
              </tr>
            ))}
            {!logs.length && (
              <tr>
                <td colSpan={5} className="muted">
                  {t('logs.noLogs')}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
