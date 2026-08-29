import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { useAuth } from '../auth/AuthContext';
import { formatBytes, formatOtaProgress } from '../utils/format';
import type { Device, Firmware, OTAJob, OTANotifyResult, Product } from '../types';

export function OtaPage() {
  const { settings } = useAuth();
  const [products, setProducts] = useState<Product[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [firmware, setFirmware] = useState<Firmware[]>([]);
  const [jobs, setJobs] = useState<OTAJob[]>([]);
  const [form, setForm] = useState({ product_id: '', version: '', url: '', sha256: '', size: '', module: 'main' });
  const [file, setFile] = useState<File | null>(null);
  const [fileKey, setFileKey] = useState(0);
  const [notifyDev, setNotifyDev] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState('');
  const [note, setNote] = useState('');
  const [error, setError] = useState('');
  const publicBase = settings?.public_base_url || '';
  const publicLooksLocal = /localhost|127\.0\.0\.1/i.test(publicBase);

  const load = useCallback(async () => {
    try {
      const [prods, devs, fws, js] = await Promise.all([
        api<Product[]>('/products'),
        api<Device[]>('/devices'),
        api<Firmware[]>('/ota/firmwares'),
        api<OTAJob[]>('/ota/jobs'),
      ]);
      setProducts(prods);
      setDevices(devs);
      setFirmware(fws);
      setJobs(js);
      setForm((f) => (!f.product_id && prods[0] ? { ...f, product_id: prods[0].product_id } : f));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'ota.progress' || msg.event === 'device.status') void load();
    });
  }, [load]);

  const existingSameVersion = useMemo(() => {
    const mod = (form.module || 'main').trim() || 'main';
    const ver = form.version.trim();
    if (!form.product_id || !ver) return undefined;
    return firmware.find((f) => f.product_id === form.product_id && (f.module || 'main') === mod && f.version === ver);
  }, [firmware, form.module, form.product_id, form.version]);

  const devicesByProduct = useMemo(() => {
    const m: Record<string, Device[]> = {};
    for (const d of devices) {
      (m[d.product_id] ||= []).push(d);
    }
    return m;
  }, [devices]);

  const summarize = (r: OTANotifyResult) => {
    const parts = [`started ${r.started}`, `skipped ${r.skipped}`];
    if (r.failed) parts.push(`failed ${r.failed}`);
    if (r.errors?.length) parts.push(r.errors.slice(0, 3).join('; '));
    setNote(parts.join(' · '));
  };

  const notify = async (fw: Firmware, deviceId?: number) => {
    const key = `n-${fw.id}-${deviceId || 'all'}`;
    setBusy(key);
    try {
      const body: { device_id?: number } = {};
      if (deviceId) body.device_id = deviceId;
      summarize(await api<OTANotifyResult>(`/ota/firmwares/${fw.id}/notify`, { method: 'POST', body: JSON.stringify(body) }));
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const toggleAuto = async (p: Product) => {
    setBusy(`auto-${p.id}`);
    try {
      await api(`/products/${p.id}`, {
        method: 'PUT',
        body: JSON.stringify({ name: p.name, description: p.description || '', auto_ota: !p.auto_ota }),
      });
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const removeFirmware = async (fw: Firmware) => {
    if (!confirm(`Delete firmware ${fw.product_id} ${fw.module}:${fw.version}? Hosted file and OTA jobs for this image will be removed.`)) return;
    setBusy(`del-${fw.id}`);
    try {
      await api(`/ota/firmwares/${fw.id}`, { method: 'DELETE' });
      setNote(`Deleted ${fw.version}`);
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const cancelJob = async (j: OTAJob) => {
    try {
      await api(`/devices/${j.device_pk}/ota/cancel`, {
        method: 'POST',
        body: JSON.stringify({ module: j.module || 'main', reason: 'cancelled by operator' }),
      });
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div>
      <h1 className="h3 mb-2">OTA</h1>
      <p className="muted mb-3">
        Upload a <span className="mono">.bin</span> here — platform stores it and gives a public download URL for ESP OTA.
        Device reports <span className="code">v1/o/ver</span>, can ask <span className="code">v1/o/q</span>. Operator pushes{' '}
        <span className="code">v1/o/upd</span> to one device or the whole product. Auto-OTA pushes latest firmware when a
        device comes online or reports a version.
      </p>
      {publicLooksLocal && (
        <div className="alert alert-warning py-2">
          <span className="mono">PUBLIC_BASE_URL</span> is <span className="mono">{publicBase}</span>. ESP on the LAN cannot
          download from localhost — set it to the host IP the device can reach (e.g. <span className="mono">http://192.168.1.100:8080</span>).
        </div>
      )}
      {error && <div className="alert alert-danger py-2">{error}</div>}
      {note && <div className="alert alert-info py-2">{note}</div>}

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Auto-OTA by product</h2>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Product</th>
                <th>Devices</th>
                <th>Auto-OTA</th>
              </tr>
            </thead>
            <tbody>
              {products.map((p) => (
                <tr key={p.id}>
                  <td className="mono">
                    <Link to={`/products/${p.id}`}>{p.product_id}</Link>
                    <div className="small muted">{p.name}</div>
                  </td>
                  <td>{(devicesByProduct[p.product_id] || []).length}</td>
                  <td>
                    <div className="form-check form-switch mb-0">
                      <input
                        className="form-check-input"
                        type="checkbox"
                        checked={!!p.auto_ota}
                        disabled={busy === `auto-${p.id}`}
                        onChange={() => void toggleAuto(p)}
                      />
                      <label className="form-check-label small">{p.auto_ota ? 'on' : 'off'}</label>
                    </div>
                  </td>
                </tr>
              ))}
              {!products.length && (
                <tr>
                  <td colSpan={3} className="muted">
                    No products
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Firmwares</h2>
        <div className="table-responsive mb-3">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Product</th>
                <th>Module</th>
                <th>Version</th>
                <th>Size</th>
                <th>Download</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {firmware.map((f) => (
                <tr key={f.id}>
                  <td className="mono">{f.product_id}</td>
                  <td>{f.module}</td>
                  <td className="mono">{f.version}</td>
                  <td>{formatBytes(f.size)}</td>
                  <td className="small">
                    {f.hosted ? <span className="badge text-bg-secondary me-1">hosted</span> : <span className="badge text-bg-light me-1">external</span>}
                    <a className="mono" href={f.url} target="_blank" rel="noreferrer">
                      {f.url}
                    </a>
                    <button
                      type="button"
                      className="btn btn-link btn-sm py-0"
                      onClick={() => {
                        void navigator.clipboard.writeText(f.url).then(
                          () => setNote('Copied download URL'),
                          () => setNote(f.url),
                        );
                      }}
                    >
                      Copy
                    </button>
                  </td>
                  <td className="text-nowrap">
                    <button
                      type="button"
                      className="btn btn-primary btn-sm me-1"
                      disabled={!!busy}
                      onClick={() => {
                        if (!confirm(`Notify all ${f.product_id} devices about ${f.version}?`)) return;
                        void notify(f);
                      }}
                    >
                      Notify product
                    </button>
                    <div className="d-inline-flex gap-1 align-items-center">
                      <select
                        className="form-select form-select-sm"
                        style={{ width: 140 }}
                        value={notifyDev[f.id] || ''}
                        onChange={(e) => setNotifyDev((s) => ({ ...s, [f.id]: e.target.value }))}
                      >
                        <option value="">device…</option>
                        {(devicesByProduct[f.product_id] || []).map((d) => (
                          <option key={d.id} value={d.id}>
                            {d.device_id}
                            {d.fw_version ? ` (${d.fw_version})` : ''}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        className="btn btn-outline-secondary btn-sm"
                        disabled={!!busy || !notifyDev[f.id]}
                        onClick={() => void notify(f, Number(notifyDev[f.id]))}
                      >
                        Notify
                      </button>
                    </div>
                    <button
                      type="button"
                      className="btn btn-outline-danger btn-sm ms-1"
                      disabled={!!busy}
                      onClick={() => void removeFirmware(f)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
              {!firmware.length && (
                <tr>
                  <td colSpan={6} className="muted">
                    No firmware registered
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <h3 className="h6 text-uppercase muted">Upload firmware</h3>
        <p className="small muted mb-2">
          Upload needs product, version and a <span className="mono">.bin</span>. Same version on the same module
          replaces the previous image (new download URL).
        </p>
        <div className="row g-2 align-items-end mb-2">
          <div className="col-md-2">
            <label className="form-label small mb-1">Product</label>
            <select className="form-select" value={form.product_id} onChange={(e) => setForm({ ...form, product_id: e.target.value })}>
              {products.map((p) => (
                <option key={p.id} value={p.product_id}>
                  {p.product_id}
                </option>
              ))}
            </select>
          </div>
          <div className="col-md-1">
            <label className="form-label small mb-1">Module</label>
            <input className="form-control" placeholder="main" value={form.module} onChange={(e) => setForm({ ...form, module: e.target.value })} />
          </div>
          <div className="col-md-2">
            <label className="form-label small mb-1">Version</label>
            <input className="form-control" placeholder="1.2.0" value={form.version} onChange={(e) => setForm({ ...form, version: e.target.value })} />
          </div>
          <div className="col-md-4">
            <label className="form-label small mb-1">File (.bin)</label>
            <input
              key={fileKey}
              className="form-control"
              type="file"
              accept=".bin,.img,application/octet-stream"
              onChange={(e) => setFile(e.target.files?.[0] || null)}
            />
          </div>
          <div className="col-md-2">
            <button
              type="button"
              className="btn btn-primary w-100"
              disabled={!!busy || !file || !form.product_id || !form.version.trim()}
              onClick={async () => {
                if (!file) return;
                if (existingSameVersion && !confirm(`Replace ${form.product_id} ${form.module || 'main'}:${form.version.trim()}? Download URL will change.`)) {
                  return;
                }
                setBusy('upload');
                try {
                  const fd = new FormData();
                  fd.append('file', file);
                  fd.append('product_id', form.product_id);
                  fd.append('version', form.version.trim());
                  fd.append('module', form.module.trim() || 'main');
                  const fw = await api<Firmware>('/ota/firmwares/upload', { method: 'POST', body: fd });
                  setForm((f) => ({ ...f, url: '', sha256: '', size: '' }));
                  setFile(null);
                  setFileKey((k) => k + 1);
                  setNote(`${existingSameVersion ? 'Replaced' : 'Uploaded'} ${fw.version} · ${fw.url}`);
                  await load();
                } catch (e) {
                  alert(e instanceof Error ? e.message : String(e));
                } finally {
                  setBusy('');
                }
              }}
            >
              {existingSameVersion ? 'Replace' : 'Upload'}
            </button>
          </div>
        </div>
        {file && <div className="small muted mb-3">{file.name} · {formatBytes(file.size)}</div>}

        <h3 className="h6 text-uppercase muted">Or register external URL</h3>
        <div className="row g-2">
          <div className="col-md-3">
            <input className="form-control" placeholder="https://…/firmware.bin" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} />
          </div>
          <div className="col-md-3">
            <input className="form-control" placeholder="sha256" value={form.sha256} onChange={(e) => setForm({ ...form, sha256: e.target.value })} />
          </div>
          <div className="col-md-2">
            <input className="form-control" type="number" placeholder="size" value={form.size} onChange={(e) => setForm({ ...form, size: e.target.value })} />
          </div>
          <div className="col-md-2">
            <button
              type="button"
              className="btn btn-outline-secondary w-100"
              disabled={!!busy || !form.url || !form.product_id || !form.version}
              onClick={async () => {
                setBusy('register');
                try {
                  await api('/ota/firmwares', {
                    method: 'POST',
                    body: JSON.stringify({
                      product_id: form.product_id,
                      version: form.version,
                      url: form.url,
                      sha256: form.sha256,
                      size: Number(form.size) || 0,
                      module: form.module || 'main',
                    }),
                  });
                  setForm((f) => ({ ...f, version: '', url: '', sha256: '', size: '' }));
                  setNote('Firmware registered');
                  await load();
                } catch (e) {
                  alert(e instanceof Error ? e.message : String(e));
                } finally {
                  setBusy('');
                }
              }}
            >
              Register URL
            </button>
          </div>
        </div>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Jobs</h2>
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Time</th>
                <th>Device</th>
                <th>Firmware</th>
                <th>Status</th>
                <th>Progress</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {jobs.map((j) => (
                <tr key={j.id}>
                  <td className="mono small">{new Date(j.created_at).toLocaleString()}</td>
                  <td className="mono">
                    <Link to={`/devices/${j.device_pk}`}>
                      {j.product_id}/{j.device_id}
                    </Link>
                  </td>
                  <td className="mono">
                    {j.module}:{j.firmware_version}
                  </td>
                  <td className="mono small">{j.status}</td>
                  <td className="small">{formatOtaProgress(j)}</td>
                  <td>
                    {(j.status === 'pending' || j.status === 'running') && (
                      <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => void cancelJob(j)}>
                        Cancel
                      </button>
                    )}
                  </td>
                </tr>
              ))}
              {!jobs.length && (
                <tr>
                  <td colSpan={6} className="muted">
                    No OTA jobs yet
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
