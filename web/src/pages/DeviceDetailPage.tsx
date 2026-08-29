import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { AttrForm } from '../components/AttrForm';
import { CredsModal } from '../components/CredsModal';
import { MetricCards } from '../components/MetricCards';
import { RpcConsole } from '../components/RpcConsole';
import { SeverityBadge, StatusBadge } from '../components/StatusBadge';
import { DevicePlugins } from '../components/DevicePlugins';
import { formatOtaProgress } from '../utils/format';
import type { DeviceCredentials, DeviceDetail, Firmware, OTAJob } from '../types';

export function DeviceDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [detail, setDetail] = useState<DeviceDetail | null>(null);
  const [creds, setCreds] = useState<DeviceCredentials | null>(null);
  const [error, setError] = useState('');
  const [firmwares, setFirmwares] = useState<Firmware[]>([]);
  const [otaJobs, setOtaJobs] = useState<OTAJob[]>([]);
  const [otaFw, setOtaFw] = useState('');
  const [otaBusy, setOtaBusy] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      const detail = await api<DeviceDetail>(`/devices/${id}`);
      setDetail(detail);
      setError('');
      try {
        const [fws, jobs] = await Promise.all([
          api<Firmware[]>(`/ota/firmwares?product_id=${encodeURIComponent(detail.device.product_id)}`),
          api<OTAJob[]>(`/ota/jobs?device_id=${id}`),
        ]);
        setFirmwares(fws);
        setOtaJobs(jobs);
        setOtaFw((prev) => prev || (fws[0] ? String(fws[0].id) : ''));
      } catch {
        /* OTA panel is optional */
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [id]);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status' || msg.event === 'telemetry' || msg.event === 'event' || msg.event === 'mqtt' || msg.event === 'ota.progress') void load();
    });
  }, [load]);

  if (!detail && !error) return <p className="muted">Loading…</p>;
  if (error && !detail) return <div className="alert alert-danger">{error}</div>;
  if (!detail) return null;

  const d = detail.device;
  const c = detail.connection;

  return (
    <div>
      <div className="d-flex flex-wrap align-items-center gap-2 mb-3">
        <button type="button" className="btn btn-outline-secondary btn-sm flex-shrink-0" onClick={() => navigate('/devices')}>
          ← Devices
        </button>
        <h1 className="h4 mb-0" style={{ minWidth: 0 }}>
          {d.product_id} / {d.device_id} <StatusBadge status={d.status} />
        </h1>
        {detail.product?.id ? (
          <Link className="muted small ms-auto flex-shrink-0" to={`/products/${detail.product.id}`}>
            Edit product schema
          </Link>
        ) : null}
      </div>

      <div className="row g-3">
        <div className="col-lg-6">
          <div className="panel">
            <h2 className="h6 text-uppercase muted">Connection</h2>
            <div className="mono small">
              <div>client_id: {c.client_id}</div>
              <div>username: {c.username || c.client_id}</div>
              <div>
                broker: {c.broker_host}:{c.broker_port}
              </div>
              {c.broker_ws_url ? (
                <div>
                  websocket: {c.broker_ws_url}
                </div>
              ) : null}
              <div>topics: {c.topic_style || 'auto'} (detected from MQTT prefix)</div>
              <div>token_prefix: {c.token_prefix}…</div>
              <div>
                mqtt session:{' '}
                {c.mqtt_session ? (
                  <span style={{ color: 'var(--ok)' }}>
                    connected from {c.mqtt_session.remote || '?'}
                    {c.mqtt_session.subscriptions?.length ? ` · ${c.mqtt_session.subscriptions.join(', ')}` : ''}
                  </span>
                ) : (
                  <span className="muted">not connected</span>
                )}
              </div>
              <div className="muted">{c.note}</div>
            </div>
            <div className="d-flex flex-wrap gap-2 mt-3">
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={async () => {
                  if (!confirm('Rotate token? Device must be updated with the new credential.')) return;
                  try {
                    setCreds(await api<DeviceCredentials>(`/devices/${d.id}/token/rotate`, { method: 'POST', body: '{}' }));
                  } catch (e) {
                    alert(e instanceof Error ? e.message : String(e));
                  }
                }}
              >
                Rotate token
              </button>
              <button
                type="button"
                className="btn btn-danger btn-sm"
                onClick={async () => {
                  if (!confirm('Delete device?')) return;
                  try {
                    await api(`/devices/${d.id}`, { method: 'DELETE' });
                    navigate('/devices');
                  } catch (e) {
                    alert(e instanceof Error ? e.message : String(e));
                  }
                }}
              >
                Delete device
              </button>
            </div>
          </div>
        </div>
        <div className="col-lg-6">
          <div className="panel">
            <h2 className="h6 text-uppercase muted">Device metrics</h2>
            <MetricCards latest={detail.latest_telemetry} product={detail.product} />
          </div>
        </div>
      </div>

      <AttrForm
        product={detail.product}
        reported={detail.reported}
        desired={detail.desired}
        onSave={async (attrs) => {
          await api(`/devices/${d.id}/attributes/desired`, { method: 'PATCH', body: JSON.stringify(attrs) });
          await load();
        }}
      />

      <div className="panel">
        <h2 className="h6 text-uppercase muted">OTA</h2>
        <div className="d-flex flex-wrap align-items-center gap-3 mb-2">
          <div className="small">
            firmware: <span className="mono">{d.fw_version || '—'}</span>
          </div>
          <div className="form-check mb-0">
            <input
              className="form-check-input"
              type="checkbox"
              id="dev-auto-ota"
              checked={d.auto_ota !== false}
              onChange={async (e) => {
                try {
                  await api(`/devices/${d.id}`, {
                    method: 'PUT',
                    body: JSON.stringify({ name: d.name, description: d.description || '', auto_ota: e.target.checked }),
                  });
                  await load();
                } catch (err) {
                  alert(err instanceof Error ? err.message : String(err));
                }
              }}
            />
            <label className="form-check-label small" htmlFor="dev-auto-ota">
              Auto-OTA (device)
            </label>
          </div>
        </div>
        <div className="row g-2 align-items-end mb-2">
          <div className="col-md-6">
            <label className="form-label small muted mb-0">Firmware</label>
            <select className="form-select form-select-sm" value={otaFw} onChange={(e) => setOtaFw(e.target.value)}>
              {firmwares.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.module}:{f.version}
                </option>
              ))}
            </select>
          </div>
          <div className="col-md-6 d-flex gap-2">
            <button
              type="button"
              className="btn btn-primary btn-sm"
              disabled={otaBusy || !otaFw || d.status !== 'online'}
              onClick={async () => {
                setOtaBusy(true);
                try {
                  await api(`/devices/${d.id}/ota`, { method: 'POST', body: JSON.stringify({ firmware_id: Number(otaFw) }) });
                  await load();
                } catch (err) {
                  alert(err instanceof Error ? err.message : String(err));
                } finally {
                  setOtaBusy(false);
                }
              }}
            >
              Notify this device
            </button>
            <button
              type="button"
              className="btn btn-outline-secondary btn-sm"
              disabled={otaBusy}
              onClick={async () => {
                setOtaBusy(true);
                try {
                  await api(`/devices/${d.id}/ota/cancel`, { method: 'POST', body: JSON.stringify({ reason: 'cancelled from device page' }) });
                  await load();
                } catch (err) {
                  alert(err instanceof Error ? err.message : String(err));
                } finally {
                  setOtaBusy(false);
                }
              }}
            >
              Cancel
            </button>
          </div>
        </div>
        {d.status !== 'online' && <p className="small muted mb-2">Device offline — job will stay pending until it reconnects.</p>}
        {!!otaJobs.length && (
          <div className="table-responsive">
            <table className="table table-sm mb-0">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Version</th>
                  <th>Status</th>
                  <th>Progress</th>
                </tr>
              </thead>
              <tbody>
                {otaJobs.slice(0, 8).map((j) => (
                  <tr key={j.id}>
                    <td className="mono small">{new Date(j.created_at).toLocaleString()}</td>
                    <td className="mono">{j.firmware_version}</td>
                    <td>{j.status}</td>
                    <td className="small">{formatOtaProgress(j)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <DevicePlugins deviceId={d.id} online={d.status === 'online'} />

      <div className="panel">
        <h2 className="h6 text-uppercase muted">RPC</h2>
        <RpcConsole product={detail.product} deviceId={d.id} online={d.status === 'online'} />
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Recent events</h2>
        <div className="table-responsive">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>Time</th>
                <th>Severity</th>
                <th>Code</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {(detail.recent_events || []).map((e, i) => (
                <tr key={i}>
                  <td className="mono">{new Date(e.ts).toLocaleString()}</td>
                  <td>
                    <SeverityBadge value={e.severity} />
                  </td>
                  <td className="mono">{e.code}</td>
                  <td>{e.message || ''}</td>
                </tr>
              ))}
              {!(detail.recent_events || []).length && (
                <tr>
                  <td colSpan={4} className="muted">
                    No events
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
