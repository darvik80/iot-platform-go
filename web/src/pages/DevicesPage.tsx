import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { CredsModal } from '../components/CredsModal';
import { StatusBadge } from '../components/StatusBadge';
import { errorMsg, useToast } from '../components/Toast';
import { useT } from '../i18n/I18nContext';
import type { Device, DeviceCredentials, Product } from '../types';

export function DevicesPage() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [q, setQ] = useState('');
  const [showAdd, setShowAdd] = useState(false);
  const [creds, setCreds] = useState<DeviceCredentials | null>(null);
  const [error, setError] = useState('');
  const [form, setForm] = useState({ product_id: '', device_id: '', name: '' });
  const toast = useToast();
  const t = useT();

  const load = useCallback(async () => {
    try {
      const [devs, prods] = await Promise.all([api<Device[]>('/devices'), api<Product[]>('/products')]);
      setDevices(devs);
      setProducts(prods);
      setForm((f) => (!f.product_id && prods[0] ? { ...f, product_id: prods[0].product_id } : f));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status' || msg.event === 'telemetry' || msg.event === 'event' || msg.event === 'mqtt') void load();
    });
  }, [load]);

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return devices;
    return devices.filter(
      (d) =>
        d.device_id.toLowerCase().includes(s) ||
        d.product_id.toLowerCase().includes(s) ||
        (d.name || '').toLowerCase().includes(s),
    );
  }, [devices, q]);

  const create = async () => {
    try {
      const res = await api<{ credentials: DeviceCredentials }>('/devices', {
        method: 'POST',
        body: JSON.stringify({
          product_id: form.product_id,
          device_id: form.device_id,
          name: form.name,
        }),
      });
      setCreds(res.credentials);
      setShowAdd(false);
      await load();
    } catch (e) {
      toast.error(errorMsg(e));
    }
  };

  return (
    <div>
      <h1 className="h3 mb-3">{t('devices.title')}</h1>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="d-flex flex-wrap gap-2 mb-3">
        <input className="form-control" style={{ maxWidth: 240, minWidth: 0, flex: '1 1 160px' }} placeholder={t('common.search')} value={q} onChange={(e) => setQ(e.target.value)} />
        <button type="button" className="btn btn-primary flex-shrink-0" onClick={() => setShowAdd((v) => !v)}>
          {t('devices.addDevice')}
        </button>
      </div>

      {showAdd && (
        <div className="panel">
          <h2 className="h6 text-uppercase muted">{t('devices.registerDevice')}</h2>
          <div className="row g-2">
            <div className="col-md-3">
              <select
                className="form-select"
                value={form.product_id}
                onChange={(e) => setForm({ ...form, product_id: e.target.value })}
              >
                {products.map((p) => (
                  <option key={p.id} value={p.product_id}>
                    {p.name} ({p.product_id})
                  </option>
                ))}
              </select>
            </div>
            <div className="col-md-3">
              <input className="form-control" placeholder="device_id" value={form.device_id} onChange={(e) => setForm({ ...form, device_id: e.target.value })} />
            </div>
            <div className="col-md-4">
              <input className="form-control" placeholder={t('common.displayName')} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="col-md-2">
              <button type="button" className="btn btn-primary w-100" onClick={create}>
                {t('common.create')}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="panel">
        <div className="table-responsive">
          <table className="table table-hover align-middle">
            <thead>
              <tr>
                <th>{t('common.product')}</th>
                <th>{t('common.device')}</th>
                <th>{t('common.name')}</th>
                <th>{t('devices.fw')}</th>
                <th>{t('common.status')}</th>
                <th>{t('mqtt.lastSeen')}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((d) => (
                <tr key={d.id}>
                  <td className="mono">{d.product_id}</td>
                  <td className="mono">
                    <Link to={`/devices/${d.id}`}>{d.device_id}</Link>
                  </td>
                  <td>{d.name}</td>
                  <td className="mono small">{d.fw_version || '—'}</td>
                  <td>
                    <StatusBadge status={d.status} />
                  </td>
                  <td className="mono">{d.last_seen_at ? new Date(d.last_seen_at).toLocaleString() : '—'}</td>
                </tr>
              ))}
              {!filtered.length && (
                <tr>
                  <td colSpan={6} className="muted">
                    {t('devices.noDevices')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {creds && <CredsModal creds={creds} onClose={() => setCreds(null)} />}
    </div>
  );
}
