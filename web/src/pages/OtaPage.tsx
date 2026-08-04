import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Firmware } from '../types';

export function OtaPage() {
  const [firmware, setFirmware] = useState<Firmware[]>([]);
  const [form, setForm] = useState({ product_id: '', version: '', url: '', sha256: '', size: '' });

  const load = () => api<Firmware[]>('/ota/firmwares').then(setFirmware);

  useEffect(() => {
    void load().catch((e) => alert(e.message));
  }, []);

  return (
    <div>
      <h1 className="h3 mb-3">OTA firmwares</h1>
      <div className="panel table-responsive">
        <table className="table table-sm">
          <thead>
            <tr>
              <th>Product</th>
              <th>Module</th>
              <th>Version</th>
              <th>Size</th>
              <th>URL</th>
            </tr>
          </thead>
          <tbody>
            {firmware.map((f, i) => (
              <tr key={i}>
                <td>{f.product_id}</td>
                <td>{f.module}</td>
                <td className="mono">{f.version}</td>
                <td>{f.size}</td>
                <td className="mono">{f.url}</td>
              </tr>
            ))}
            {!firmware.length && <tr><td colSpan={5} className="muted">No firmware</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="panel">
        <h2 className="h6 text-uppercase muted">Register firmware</h2>
        <div className="row g-2">
          {(['product_id', 'version', 'url', 'sha256'] as const).map((k) => (
            <div className="col-md-3" key={k}>
              <input className="form-control" placeholder={k} value={form[k]} onChange={(e) => setForm({ ...form, [k]: e.target.value })} />
            </div>
          ))}
          <div className="col-md-2">
            <input className="form-control" type="number" placeholder="size" value={form.size} onChange={(e) => setForm({ ...form, size: e.target.value })} />
          </div>
          <div className="col-md-2">
            <button
              type="button"
              className="btn btn-primary w-100"
              onClick={async () => {
                try {
                  await api('/ota/firmwares', {
                    method: 'POST',
                    body: JSON.stringify({
                      product_id: form.product_id,
                      version: form.version,
                      url: form.url,
                      sha256: form.sha256,
                      size: Number(form.size) || 0,
                      module: 'main',
                    }),
                  });
                  setForm({ product_id: '', version: '', url: '', sha256: '', size: '' });
                  await load();
                } catch (e) {
                  alert(e instanceof Error ? e.message : String(e));
                }
              }}
            >
              Register
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
