import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { dialectOf } from '../dialects';
import { AttrForm } from '../components/AttrForm';
import { ConfirmModal } from '../components/ConfirmModal';
import { CredsModal } from '../components/CredsModal';
import { MetricCards } from '../components/MetricCards';
import { RpcConsole } from '../components/RpcConsole';
import { SeverityBadge, StatusBadge } from '../components/StatusBadge';
import { DevicePlugins } from '../components/DevicePlugins';
import { errorMsg, useToast } from '../components/Toast';
import { formatOtaProgress } from '../utils/format';
import { useT } from '../i18n/I18nContext';
import type { DeviceCredentials, DeviceDetail, Firmware, OTAJob, DashboardPanel, DashboardResponse } from '../types';

type DetailTab = 'overview' | 'metrics';

export function DeviceDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const t = useT();
  const [detail, setDetail] = useState<DeviceDetail | null>(null);
  const [creds, setCreds] = useState<DeviceCredentials | null>(null);
  const [error, setError] = useState('');
  const [firmwares, setFirmwares] = useState<Firmware[]>([]);
  const [otaJobs, setOtaJobs] = useState<OTAJob[]>([]);
  const [otaFw, setOtaFw] = useState('');
  const [otaBusy, setOtaBusy] = useState(false);
  const [tab, setTab] = useState<DetailTab>('overview');
  const [dashboard, setDashboard] = useState<DashboardPanel[] | null>(null);
  const [dashLoading, setDashLoading] = useState(false);
  const [confirmAction, setConfirmAction] = useState<'rotateToken' | 'deleteDevice' | null>(null);

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

  const loadDashboard = useCallback(async () => {
    if (!id) return;
    setDashLoading(true);
    try {
      const resp = await api<DashboardResponse>(`/devices/${id}/dashboard`);
      setDashboard(resp.panels);
    } catch {
      setDashboard([]);
    } finally {
      setDashLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (tab === 'metrics' && dashboard === null && !dashLoading) {
      void loadDashboard();
    }
  }, [tab, dashboard, dashLoading, loadDashboard]);

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
  const connDialect = dialectOf(c.topic_style);
  const devDialect = dialectOf(d.topic_style);

  return (
    <div>
      <div className="d-flex flex-wrap align-items-center gap-2 mb-3">
        <button type="button" className="btn btn-outline-secondary btn-sm flex-shrink-0" onClick={() => navigate('/devices')}>
          ← {t('devices.title')}
        </button>
        <h1 className="h4 mb-0" style={{ minWidth: 0 }}>
          {d.product_id} / {d.device_id} <StatusBadge status={d.status} />
        </h1>
        {detail.product?.id ? (
          <Link className="muted small ms-auto flex-shrink-0" to={`/products/${detail.product.id}`}>
            {t('device.editProductSchema')}
          </Link>
        ) : null}
      </div>

      {/* Tab navigation */}
      <ul className="nav nav-tabs mb-3">
        <li className="nav-item">
          <button className={`nav-link${tab === 'overview' ? ' active' : ''}`} onClick={() => setTab('overview')}>
            {t('device.overview')}
          </button>
        </li>
        <li className="nav-item">
          <button className={`nav-link${tab === 'metrics' ? ' active' : ''}`} onClick={() => { setTab('metrics'); }}>
            {t('device.metricsPlus')}
          </button>
        </li>
      </ul>

      {tab === 'overview' && (<>
      <div className="row g-3">
        <div className="col-lg-6">
          <div className="panel">
            <h2 className="h6 text-uppercase muted">{t('device.connection')}</h2>
            <div className="mono small">
              {c.client_id ? <div>client_id: {c.client_id}</div> : null}
              <div>
                username:{' '}
                {connDialect.credential === 'token'
                  ? `${c.token_prefix}… (${t('devices.usernameToken')})`
                  : c.username || c.client_id}
              </div>
              <div>
                broker: {c.broker_host}:{c.broker_port}
              </div>
              {c.broker_ws_url ? (
                <div>
                  websocket: {c.broker_ws_url}
                </div>
              ) : null}
              <div>topics: {t(connDialect.topicsKey)}</div>
              {connDialect.credential === 'native' && (
                <div>token_prefix: {c.token_prefix}…</div>
              )}
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
                onClick={() => setConfirmAction('rotateToken')}
              >
                {devDialect.credential === 'secret' ? t('device.rotateSecret') : t('device.rotateToken')}
              </button>
              <button
                type="button"
                className="btn btn-danger btn-sm"
                onClick={() => setConfirmAction('deleteDevice')}
              >
                {t('device.deleteDevice')}
              </button>
            </div>
          </div>
        </div>
        <div className="col-lg-6">
          <div className="panel">
            <h2 className="h6 text-uppercase muted">{t('device.tags')}</h2>
            <TagsEditor
              tags={d.tags || []}
              onSave={async (tags) => {
                try {
                  await api(`/devices/${d.id}`, {
                    method: 'PUT',
                    body: JSON.stringify({ name: d.name, description: d.description || '', tags }),
                  });
                  await load();
                } catch (err) {
                  toast.error(errorMsg(err));
                }
              }}
            />
          </div>
        </div>
      </div>

      <div className="row g-3 mt-1">
        <div className="col-lg-6">
          <div className="panel">
            <h2 className="h6 text-uppercase muted">{t('device.metrics')}</h2>
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
                  toast.error(errorMsg(err));
                }
              }}
            />
            <label className="form-check-label small" htmlFor="dev-auto-ota">
              {t('device.autoOta')}
            </label>
          </div>
        </div>
        <div className="row g-2 align-items-end mb-2">
          <div className="col-md-6">
            <label className="form-label small muted mb-0">{t('device.firmware')}</label>
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
                  toast.error(errorMsg(err));
                } finally {
                  setOtaBusy(false);
                }
              }}
            >
              {t('device.notifyThisDevice')}
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
                  toast.error(errorMsg(err));
                } finally {
                  setOtaBusy(false);
                }
              }}
            >
              {t('common.cancel')}
            </button>
          </div>
        </div>
        {d.status !== 'online' && <p className="small muted mb-2">{t('device.deviceOffline')}</p>}
        {!!otaJobs.length && (
          <div className="table-responsive">
            <table className="table table-sm mb-0">
              <thead>
                <tr>
                  <th>{t('common.time')}</th>
                  <th>{t('common.version')}</th>
                  <th>{t('common.status')}</th>
                  <th>{t('ota.progress')}</th>
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
        <h2 className="h6 text-uppercase muted">{t('device.recentEvents')}</h2>
        <div className="table-responsive">
          <table className="table table-sm">
            <thead>
              <tr>
                <th>{t('common.time')}</th>
                <th>{t('overview.severity')}</th>
                <th>{t('overview.code')}</th>
                <th>{t('common.message')}</th>
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
                    {t('overview.noEvents')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
      </>)}

      {tab === 'metrics' && (
        <div>
          {dashLoading && <p className="muted">{t('device.loadingDashboard')}</p>}
          {dashboard && !dashboard.length && <p className="muted">{t('device.noDashboardPanels')}</p>}
          {dashboard && !!dashboard.length && (
            <div className="row g-3">
              {dashboard.map((p) => (
                <div key={p.id} className="col-lg-6">
                  <div className="panel h-100">
                    <h2 className="h6 text-uppercase muted">{p.title}</h2>
                    <PanelRenderer panel={p} />
                  </div>
                </div>
              ))}
            </div>
          )}
          <div className="d-flex gap-2 mt-3">
            <button className="btn btn-outline-secondary btn-sm" onClick={() => { setDashboard(null); void loadDashboard(); }}>
              {t('common.refresh')}
            </button>
          </div>
        </div>
      )}

      {creds && <CredsModal creds={creds} onClose={() => setCreds(null)} />}

      {confirmAction === 'rotateToken' && (
        <ConfirmModal
          message={devDialect.credential === 'secret' ? t('device.rotateSecretConfirm') : t('device.rotateTokenConfirm')}
          danger={false}
          onConfirm={async () => {
            setConfirmAction(null);
            try {
              setCreds(await api<DeviceCredentials>(`/devices/${d.id}/token/rotate`, { method: 'POST', body: '{}' }));
            } catch (e) {
              toast.error(errorMsg(e));
            }
          }}
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {confirmAction === 'deleteDevice' && (
        <ConfirmModal
          message={t('device.deleteConfirm')}
          danger
          onConfirm={async () => {
            setConfirmAction(null);
            try {
              await api(`/devices/${d.id}`, { method: 'DELETE' });
              navigate('/devices');
            } catch (e) {
              toast.error(errorMsg(e));
            }
          }}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  );
}

// ── Panel renderers ───────────────────────────────────────────────────────

function PanelRenderer({ panel }: { panel: DashboardPanel }) {
  switch (panel.type) {
    case 'telemetry_chart':
      return <TelemetryChart data={panel.data} />;
    case 'telemetry_gauge':
      return <TelemetryGauge data={panel.data} />;
    case 'rpc_table':
      return <RpcTable data={panel.data} />;
    case 'online_timeline':
      return <OnlineTimeline data={panel.data} />;
    case 'events_count':
      return <EventsCount data={panel.data} />;
    default:
      return <p className="muted small">Unknown panel type</p>;
  }
}

function TelemetryChart({ data }: { data: unknown }) {
  const series = data as { key: string; points: [number, number][] }[];
  if (!Array.isArray(series) || !series.length) return <p className="muted small">No data</p>;

  const allPts = series.flatMap((s) => s.points);
  if (!allPts.length) return <p className="muted small">No data</p>;

  const tsMin = Math.min(...allPts.map((p) => p[0]));
  const tsMax = Math.max(...allPts.map((p) => p[0]));
  const valMin = Math.min(...allPts.map((p) => (typeof p[1] === 'number' ? p[1] : 0)));
  const valMax = Math.max(...allPts.map((p) => (typeof p[1] === 'number' ? p[1] : 0)));
  const valRange = valMax - valMin || 1;
  const tsRange = tsMax - tsMin || 1;

  const W = 400;
  const H = 120;
  const pad = 4;
  const colors = ['#4fc3f7', '#81c784', '#ffb74d', '#e57373', '#ba68c8'];

  return (
    <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} style={{ maxHeight: 160 }}>
      {series.map((s, si) => {
        const color = colors[si % colors.length];
        const pts = s.points
          .filter((p) => typeof p[1] === 'number')
          .map((p) => {
            const x = pad + ((p[0] - tsMin) / tsRange) * (W - 2 * pad);
            const y = H - pad - ((p[1] - valMin) / valRange) * (H - 2 * pad);
            return `${x},${y}`;
          })
          .join(' ');
        return <polyline key={s.key} points={pts} fill="none" stroke={color} strokeWidth="1.5" />;
      })}
      <text x={pad} y={10} fontSize="8" fill="var(--muted)">
        {valMax.toFixed(1)}
      </text>
      <text x={pad} y={H - 2} fontSize="8" fill="var(--muted)">
        {valMin.toFixed(1)}
      </text>
    </svg>
  );
}

function TelemetryGauge({ data }: { data: unknown }) {
  const gauges = data as { key: string; current: number; min: number; max: number; avg: number }[];
  if (!Array.isArray(gauges) || !gauges.length) return <p className="muted small">No data</p>;
  return (
    <div className="d-flex flex-wrap gap-3">
      {gauges.map((g) => (
        <div key={g.key} className="text-center" style={{ minWidth: 100 }}>
          <div className="mono" style={{ fontSize: '1.5rem', fontWeight: 600 }}>
            {g.current.toFixed(1)}
          </div>
          <div className="small muted">{g.key}</div>
          <div className="small muted">
            min {g.min.toFixed(1)} / avg {g.avg.toFixed(1)} / max {g.max.toFixed(1)}
          </div>
        </div>
      ))}
    </div>
  );
}

function RpcTable({ data }: { data: unknown }) {
  const rows = data as { method: string; status: string; created_at: number; completed_at?: number | null; message?: string }[];
  if (!Array.isArray(rows) || !rows.length) return <p className="muted small">No RPC calls</p>;
  return (
    <div className="table-responsive">
      <table className="table table-sm mb-0">
        <thead>
          <tr>
            <th>Method</th>
            <th>Status</th>
            <th>Time</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 20).map((r, i) => (
            <tr key={i}>
              <td className="mono small">{r.method}</td>
              <td>
                <span className={`badge ${r.status === 'ok' ? 'bg-success' : r.status === 'pending' ? 'bg-warning' : 'bg-danger'}`}>
                  {r.status}
                </span>
              </td>
              <td className="mono small">{new Date(r.created_at).toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function OnlineTimeline({ data }: { data: unknown }) {
  const segments = data as { start: number; end: number; online: boolean }[];
  if (!Array.isArray(segments) || !segments.length) return <p className="muted small">No data</p>;

  const tsMin = Math.min(...segments.map((s) => s.start));
  const tsMax = Math.max(...segments.map((s) => s.end));
  const tsRange = tsMax - tsMin || 1;
  const W = 400;
  const H = 24;

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} style={{ maxHeight: 32, borderRadius: 4, overflow: 'hidden' }}>
        {segments.map((s, i) => {
          const x = ((s.start - tsMin) / tsRange) * W;
          const w = ((s.end - s.start) / tsRange) * W;
          return <rect key={i} x={x} y={0} width={Math.max(w, 1)} height={H} fill={s.online ? 'var(--ok)' : 'var(--muted)'} opacity={0.6} />;
        })}
      </svg>
      <div className="d-flex gap-3 mt-1 small muted">
        <span>
          <span style={{ display: 'inline-block', width: 10, height: 10, background: 'var(--ok)', borderRadius: 2, marginRight: 4, opacity: 0.6 }} />
          Online
        </span>
        <span>
          <span style={{ display: 'inline-block', width: 10, height: 10, background: 'var(--muted)', borderRadius: 2, marginRight: 4, opacity: 0.6 }} />
          Offline
        </span>
      </div>
    </div>
  );
}

function EventsCount({ data }: { data: unknown }) {
  const counts = data as { critical: number; error: number; warning: number; info: number };
  if (!counts) return <p className="muted small">No data</p>;
  const items = [
    { label: 'Critical', value: counts.critical, color: '#e57373' },
    { label: 'Error', value: counts.error, color: '#ffb74d' },
    { label: 'Warning', value: counts.warning, color: '#ffd54f' },
    { label: 'Info', value: counts.info, color: '#4fc3f7' },
  ];
  const max = Math.max(...items.map((i) => i.value), 1);
  return (
    <div className="d-flex flex-column gap-1">
      {items.map((it) => (
        <div key={it.label} className="d-flex align-items-center gap-2 small">
          <span style={{ width: 60 }}>{it.label}</span>
          <div style={{ flex: 1, background: 'var(--bg-card)', borderRadius: 3, height: 14, overflow: 'hidden' }}>
            <div style={{ width: `${(it.value / max) * 100}%`, background: it.color, height: '100%', borderRadius: 3, minWidth: it.value > 0 ? 2 : 0 }} />
          </div>
          <span className="mono" style={{ width: 30, textAlign: 'right' }}>
            {it.value}
          </span>
        </div>
      ))}
    </div>
  );
}

function TagsEditor({ tags, onSave }: { tags: string[]; onSave: (tags: string[]) => void }) {
  const [newTag, setNewTag] = useState('');
  const t = useT();

  const addTag = () => {
    const tag = newTag.trim();
    if (tag && !tags.includes(tag)) {
      onSave([...tags, tag]);
    }
    setNewTag('');
  };

  const removeTag = (tag: string) => {
    onSave(tags.filter((t) => t !== tag));
  };

  return (
    <div>
      <div className="d-flex flex-wrap gap-1 mb-2">
        {tags.length === 0 && <span className="muted small">{t('device.noTags')}</span>}
        {tags.map((tag) => (
          <span key={tag} className="badge text-bg-secondary d-inline-flex align-items-center gap-1">
            {tag}
            <button type="button" className="btn-close btn-close-white" style={{ fontSize: '0.6rem' }} onClick={() => removeTag(tag)} aria-label={`Remove ${tag}`} />
          </span>
        ))}
      </div>
      <div className="input-group input-group-sm">
        <input
          type="text"
          className="form-control"
          placeholder={t('device.addTag')}
          value={newTag}
          onChange={(e) => setNewTag(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addTag(); } }}
        />
        <button type="button" className="btn btn-outline-secondary" onClick={addTag}>
          {t('common.add')}
        </button>
      </div>
    </div>
  );
}
