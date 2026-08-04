import { useEffect, useState } from 'react';
import type { Product, ProductMethod } from '../types';
import { api } from '../api/client';

export function RpcConsole({ product, deviceId }: { product?: Product; deviceId: number }) {
  const methods = product?.methods || [];
  const [idx, setIdx] = useState(0);
  const [params, setParams] = useState<Record<string, string>>({});
  const [result, setResult] = useState('Result will appear here');
  const [busy, setBusy] = useState(false);

  const method: ProductMethod | undefined = methods[idx];

  useEffect(() => {
    const init: Record<string, string> = {};
    (method?.params || []).forEach((p) => {
      if (p.default !== undefined && p.default !== null) init[p.name] = String(p.default);
      else init[p.name] = '';
    });
    setParams(init);
  }, [method]);

  const invoke = async (m?: ProductMethod) => {
    const target = m || method;
    if (!target) return;
    setBusy(true);
    try {
      const bodyParams: Record<string, unknown> = {};
      (target.params || []).forEach((p) => {
        const raw = params[p.name] ?? '';
        const t = p.type || 'string';
        if (t === 'number') bodyParams[p.name] = raw === '' ? 0 : Number(raw);
        else if (t === 'boolean') bodyParams[p.name] = raw === 'true';
        else if (t === 'json') {
          try {
            bodyParams[p.name] = JSON.parse(raw);
          } catch {
            bodyParams[p.name] = raw;
          }
        } else bodyParams[p.name] = raw;
      });
      const res = await api(`/devices/${deviceId}/rpc`, {
        method: 'POST',
        body: JSON.stringify({ method: target.name, params: bodyParams }),
      });
      setResult(JSON.stringify(res, null, 2));
    } catch (e) {
      setResult(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (!methods.length) {
    return <p className="muted mb-0">Register methods on the product to call them here.</p>;
  }

  return (
    <div>
      <div className="d-flex flex-wrap gap-2 mb-3">
        {methods.map((m, i) => (
          <button
            key={m.name}
            type="button"
            className="btn btn-outline-secondary btn-sm"
            onClick={() => {
              setIdx(i);
              void invoke(m);
            }}
          >
            {m.label || m.name}
          </button>
        ))}
      </div>
      <div className="row g-2 mb-2">
        <div className="col-md-4">
          <label className="form-label small muted">Method</label>
          <select className="form-select form-select-sm" value={idx} onChange={(e) => setIdx(Number(e.target.value))}>
            {methods.map((m, i) => (
              <option key={m.name} value={i}>
                {m.label || m.name} ({m.name})
              </option>
            ))}
          </select>
        </div>
        {(method?.params || []).map((p) => (
          <div className="col-md-3" key={p.name}>
            <label className="form-label small muted">{p.label || p.name}</label>
            {p.type === 'boolean' ? (
              <select
                className="form-select form-select-sm"
                value={params[p.name] || 'false'}
                onChange={(e) => setParams((s) => ({ ...s, [p.name]: e.target.value }))}
              >
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            ) : (
              <input
                className="form-control form-control-sm"
                value={params[p.name] || ''}
                onChange={(e) => setParams((s) => ({ ...s, [p.name]: e.target.value }))}
              />
            )}
          </div>
        ))}
      </div>
      <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => invoke()}>
        Invoke
      </button>
      <pre className="mt-3 small muted mb-0">{result}</pre>
    </div>
  );
}
