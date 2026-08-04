import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { AttrForm } from '../components/AttrForm';
import { CredsModal } from '../components/CredsModal';
import { MetricCards } from '../components/MetricCards';
import { RpcConsole } from '../components/RpcConsole';
import { SeverityBadge, StatusBadge } from '../components/StatusBadge';
import type { DeviceCredentials, DeviceDetail } from '../types';

export function DeviceDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [detail, setDetail] = useState<DeviceDetail | null>(null);
  const [creds, setCreds] = useState<DeviceCredentials | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!id) return;
    try {
      setDetail(await api<DeviceDetail>(`/devices/${id}`));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [id]);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status' || msg.event === 'telemetry' || msg.event === 'event') void load();
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
        <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => navigate('/devices')}>
          ← Devices
        </button>
        <h1 className="h4 mb-0">
          {d.product_id} / {d.device_id} <StatusBadge status={d.status} />
        </h1>
        {detail.product?.id ? (
          <Link className="muted small ms-auto" to={`/products/${detail.product.id}`}>
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
              <div>topic_style: {c.topic_style}</div>
              <div>token_prefix: {c.token_prefix}…</div>
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
        <h2 className="h6 text-uppercase muted">RPC</h2>
        <RpcConsole product={detail.product} deviceId={d.id} />
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
