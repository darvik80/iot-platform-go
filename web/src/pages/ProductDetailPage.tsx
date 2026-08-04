import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import { MethodEditor, PRESET_METHODS, PRESET_METRICS } from '../components/MethodEditor';
import type { Product, ProductAttribute, ProductMethod, ProductMetric } from '../types';

type Draft = {
  name: string;
  description: string;
  style: string;
  methods: ProductMethod[];
  metrics: ProductMetric[];
  attributes: ProductAttribute[];
};

export function ProductDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [p, setP] = useState<Product | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState('');
  const draftRef = useRef<Draft | null>(null);
  const [addMethod, setAddMethod] = useState({ name: '', label: '', description: '' });
  const [addMetric, setAddMetric] = useState({ key: '', label: '', group: 'Sensors', format: 'number', unit: '' });
  const [addAttr, setAddAttr] = useState({
    key: '',
    label: '',
    scope: 'desired',
    type: 'number',
    unit: '',
    writable: true,
  });

  const applyProduct = useCallback((pr: Product) => {
    const next: Draft = {
      name: pr.name,
      description: pr.description || '',
      style: pr.topic_style_default || 'short',
      methods: pr.methods || [],
      metrics: pr.metrics || [],
      attributes: pr.attributes || [],
    };
    setP(pr);
    setDraft(next);
    draftRef.current = next;
  }, []);

  const patchDraft = useCallback((patch: Partial<Draft>) => {
    setDraft((prev) => {
      if (!prev) return prev;
      const next = { ...prev, ...patch };
      draftRef.current = next;
      return next;
    });
  }, []);

  const load = useCallback(async () => {
    const pr = await api<Product>(`/products/${id}`);
    applyProduct(pr);
  }, [id, applyProduct]);

  useEffect(() => {
    void load().catch((e) => alert(e.message));
  }, [load]);

  const persist = useCallback(
    async (override?: Partial<Draft>, opts?: { quiet?: boolean }) => {
      if (!p) return;
      const base = draftRef.current;
      if (!base) return;
      const payload = { ...base, ...override };
      setBusy(true);
      setStatus('');
      try {
        const updated = await api<Product>(`/products/${p.id}`, {
          method: 'PUT',
          body: JSON.stringify({
            name: payload.name,
            description: payload.description,
            topic_style_default: payload.style,
            methods: payload.methods,
            metrics: payload.metrics,
            attributes: payload.attributes,
          }),
        });
        applyProduct(updated);
        if (!opts?.quiet) setStatus('Saved');
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        setStatus('');
        alert(msg);
        throw e;
      } finally {
        setBusy(false);
      }
    },
    [p, applyProduct],
  );

  if (!p || !draft) return <p className="muted">Loading…</p>;

  const { name, description, style, methods, metrics, attributes } = draft;

  return (
    <div>
      <div className="d-flex flex-wrap gap-2 align-items-center mb-3">
        <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => navigate('/products')}>
          ← Products
        </button>
        <h1 className="h4 mb-0 mono">{p.product_id}</h1>
        <button
          type="button"
          className="btn btn-outline-secondary btn-sm"
          disabled={busy}
          onClick={async () => {
            try {
              const updated = await api<Product>(`/products/${p.id}/esp-defaults`, { method: 'POST', body: '{}' });
              applyProduct(updated);
              setStatus('ESP presets restored');
            } catch (e) {
              alert(e instanceof Error ? e.message : String(e));
            }
          }}
        >
          Reset to ESP presets
        </button>
        <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => void persist()}>
          {busy ? 'Saving…' : 'Save'}
        </button>
        {status ? <span className="small text-success">{status}</span> : null}
        <button
          type="button"
          className="btn btn-danger btn-sm"
          onClick={async () => {
            if (!confirm(`Delete product ${p.product_id}?`)) return;
            try {
              await api(`/products/${p.id}`, { method: 'DELETE' });
              navigate('/products');
            } catch (e) {
              alert(e instanceof Error ? e.message : String(e));
            }
          }}
        >
          Delete
        </button>
      </div>

      <div className="panel">
        <div className="row g-2">
          <div className="col-md-3">
            <label className="form-label small muted">Name</label>
            <input className="form-control" value={name} onChange={(e) => patchDraft({ name: e.target.value })} />
          </div>
          <div className="col-md-2">
            <label className="form-label small muted">Topic style</label>
            <select className="form-select" value={style} onChange={(e) => patchDraft({ style: e.target.value })}>
              <option value="short">short</option>
              <option value="full">full</option>
            </select>
          </div>
          <div className="col-md-7">
            <label className="form-label small muted">Description</label>
            <input className="form-control" value={description} onChange={(e) => patchDraft({ description: e.target.value })} />
          </div>
        </div>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">
          RPC methods <span className="fw-normal text-lowercase">· preset locked</span>
        </h2>
        <MethodEditor
          methods={methods}
          onChange={(next) => {
            const removed = next.length < methods.length;
            patchDraft({ methods: next });
            if (removed) {
              void persist({ methods: next }, { quiet: true }).then(() => setStatus('Method removed'));
            }
          }}
        />
        <details className="mt-2">
          <summary className="muted">+ Add method</summary>
          <div className="row g-2 mt-2">
            <div className="col-md-3">
              <input className="form-control form-control-sm mono" placeholder="setLed" value={addMethod.name} onChange={(e) => setAddMethod({ ...addMethod, name: e.target.value })} />
            </div>
            <div className="col-md-3">
              <input className="form-control form-control-sm" placeholder="Set LED" value={addMethod.label} onChange={(e) => setAddMethod({ ...addMethod, label: e.target.value })} />
            </div>
            <div className="col-md-4">
              <input className="form-control form-control-sm" placeholder="Description" value={addMethod.description} onChange={(e) => setAddMethod({ ...addMethod, description: e.target.value })} />
            </div>
            <div className="col-md-2">
              <button
                type="button"
                className="btn btn-outline-secondary btn-sm w-100"
                disabled={busy}
                onClick={() => {
                  const n = addMethod.name.trim();
                  if (!n) return alert('Method name required');
                  if (PRESET_METHODS[n]) return alert(`"${n}" is a predefined method`);
                  if (methods.some((m) => m.name === n)) return alert('Method already exists');
                  const nextMethods = [
                    ...methods,
                    {
                      name: n,
                      label: addMethod.label || n,
                      description: addMethod.description || '',
                      params: [] as ProductMethod['params'],
                    },
                  ];
                  patchDraft({ methods: nextMethods });
                  setAddMethod({ name: '', label: '', description: '' });
                  void persist({ methods: nextMethods }, { quiet: true }).then(() => setStatus(`Method "${n}" saved`));
                }}
              >
                Add & save
              </button>
            </div>
          </div>
        </details>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">
          Metrics <span className="fw-normal text-lowercase">· ESP presets locked</span>
        </h2>
        <div className="table-responsive">
          <table className="table table-sm schema-compact">
            <thead>
              <tr>
                <th>Key</th>
                <th>Label</th>
                <th>Group</th>
                <th>Format</th>
                <th>Unit</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {metrics.map((m, i) => (
                <tr key={m.key + i}>
                  <td>
                    <input
                      className="form-control form-control-sm mono"
                      value={m.key}
                      readOnly={!!PRESET_METRICS[m.key]}
                      onChange={(e) => {
                        const next = [...metrics];
                        next[i] = { ...m, key: e.target.value };
                        patchDraft({ metrics: next });
                      }}
                    />
                  </td>
                  <td>
                    <input
                      className="form-control form-control-sm"
                      value={m.label || ''}
                      onChange={(e) => {
                        const next = [...metrics];
                        next[i] = { ...m, label: e.target.value };
                        patchDraft({ metrics: next });
                      }}
                    />
                  </td>
                  <td>
                    <input
                      className="form-control form-control-sm"
                      value={m.group || ''}
                      onChange={(e) => {
                        const next = [...metrics];
                        next[i] = { ...m, group: e.target.value };
                        patchDraft({ metrics: next });
                      }}
                    />
                  </td>
                  <td>
                    <select
                      className="form-select form-select-sm"
                      value={m.format || ''}
                      onChange={(e) => {
                        const next = [...metrics];
                        next[i] = { ...m, format: e.target.value };
                        patchDraft({ metrics: next });
                      }}
                    >
                      {['', 'bytes', 'percent', 'duration', 'bool', 'rssi', 'number', 'text'].map((f) => (
                        <option key={f} value={f}>
                          {f || 'auto'}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <input
                      className="form-control form-control-sm"
                      value={m.unit || ''}
                      onChange={(e) => {
                        const next = [...metrics];
                        next[i] = { ...m, unit: e.target.value };
                        patchDraft({ metrics: next });
                      }}
                    />
                  </td>
                  <td>
                    {PRESET_METRICS[m.key] ? (
                      <span className="badge badge-info">preset</span>
                    ) : (
                      <button
                        type="button"
                        className="btn btn-outline-secondary btn-sm"
                        disabled={busy}
                        onClick={() => {
                          const next = metrics.filter((_, j) => j !== i);
                          patchDraft({ metrics: next });
                          void persist({ metrics: next }, { quiet: true }).then(() => setStatus('Metric removed'));
                        }}
                      >
                        ×
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <details>
          <summary className="muted">+ Add metric</summary>
          <div className="row g-2 mt-2">
            <div className="col-md-2">
              <input className="form-control form-control-sm mono" placeholder="key" value={addMetric.key} onChange={(e) => setAddMetric({ ...addMetric, key: e.target.value })} />
            </div>
            <div className="col-md-2">
              <input className="form-control form-control-sm" placeholder="label" value={addMetric.label} onChange={(e) => setAddMetric({ ...addMetric, label: e.target.value })} />
            </div>
            <div className="col-md-2">
              <select className="form-select form-select-sm" value={addMetric.group} onChange={(e) => setAddMetric({ ...addMetric, group: e.target.value })}>
                {['System', 'Network', 'Memory', 'Storage', 'Sensors', 'Other'].map((g) => (
                  <option key={g}>{g}</option>
                ))}
              </select>
            </div>
            <div className="col-md-2">
              <select className="form-select form-select-sm" value={addMetric.format} onChange={(e) => setAddMetric({ ...addMetric, format: e.target.value })}>
                {['', 'bytes', 'percent', 'duration', 'bool', 'rssi', 'number', 'text'].map((f) => (
                  <option key={f} value={f}>
                    {f || 'auto'}
                  </option>
                ))}
              </select>
            </div>
            <div className="col-md-2">
              <input className="form-control form-control-sm" placeholder="unit" value={addMetric.unit} onChange={(e) => setAddMetric({ ...addMetric, unit: e.target.value })} />
            </div>
            <div className="col-md-2">
              <button
                type="button"
                className="btn btn-outline-secondary btn-sm w-100"
                disabled={busy}
                onClick={() => {
                  const key = addMetric.key.trim();
                  if (!key) return alert('Metric key required');
                  if (PRESET_METRICS[key]) return alert(`"${key}" is a predefined metric`);
                  if (metrics.some((m) => m.key === key)) return alert('Metric already exists');
                  const next = [...metrics, { ...addMetric, key, label: addMetric.label || key }];
                  patchDraft({ metrics: next });
                  setAddMetric({ key: '', label: '', group: 'Sensors', format: 'number', unit: '' });
                  void persist({ metrics: next }, { quiet: true }).then(() => setStatus(`Metric "${key}" saved`));
                }}
              >
                Add & save
              </button>
            </div>
          </div>
        </details>
      </div>

      <div className="panel">
        <h2 className="h6 text-uppercase muted">Attributes</h2>
        <div className="table-responsive">
          <table className="table table-sm schema-compact">
            <thead>
              <tr>
                <th>Key</th>
                <th>Label</th>
                <th>Scope</th>
                <th>Type</th>
                <th>Unit</th>
                <th>W</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {attributes.map((a, i) => (
                <tr key={a.key + i}>
                  <td>
                    <input
                      className="form-control form-control-sm mono"
                      value={a.key}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, key: e.target.value };
                        patchDraft({ attributes: n });
                      }}
                    />
                  </td>
                  <td>
                    <input
                      className="form-control form-control-sm"
                      value={a.label || ''}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, label: e.target.value };
                        patchDraft({ attributes: n });
                      }}
                    />
                  </td>
                  <td>
                    <select
                      className="form-select form-select-sm"
                      value={a.scope || 'reported'}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, scope: e.target.value };
                        patchDraft({ attributes: n });
                      }}
                    >
                      {['reported', 'desired', 'both'].map((s) => (
                        <option key={s}>{s}</option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <select
                      className="form-select form-select-sm"
                      value={a.type || 'string'}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, type: e.target.value };
                        patchDraft({ attributes: n });
                      }}
                    >
                      {['number', 'string', 'boolean', 'json'].map((t) => (
                        <option key={t}>{t}</option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <input
                      className="form-control form-control-sm"
                      value={a.unit || ''}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, unit: e.target.value };
                        patchDraft({ attributes: n });
                      }}
                    />
                  </td>
                  <td className="text-center">
                    <input
                      type="checkbox"
                      checked={!!a.writable}
                      onChange={(e) => {
                        const n = [...attributes];
                        n[i] = { ...a, writable: e.target.checked };
                        patchDraft({ attributes: n });
                      }}
                    />
                  </td>
                  <td>
                    <button
                      type="button"
                      className="btn btn-outline-secondary btn-sm"
                      disabled={busy}
                      onClick={() => {
                        const next = attributes.filter((_, j) => j !== i);
                        patchDraft({ attributes: next });
                        void persist({ attributes: next }, { quiet: true }).then(() => setStatus('Attribute removed'));
                      }}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <details>
          <summary className="muted">+ Add attribute</summary>
          <div className="row g-2 mt-2">
            <div className="col-md-2">
              <input className="form-control form-control-sm mono" placeholder="key" value={addAttr.key} onChange={(e) => setAddAttr({ ...addAttr, key: e.target.value })} />
            </div>
            <div className="col-md-2">
              <input className="form-control form-control-sm" placeholder="label" value={addAttr.label} onChange={(e) => setAddAttr({ ...addAttr, label: e.target.value })} />
            </div>
            <div className="col-md-2">
              <select className="form-select form-select-sm" value={addAttr.scope} onChange={(e) => setAddAttr({ ...addAttr, scope: e.target.value })}>
                {['desired', 'reported', 'both'].map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </div>
            <div className="col-md-2">
              <select className="form-select form-select-sm" value={addAttr.type} onChange={(e) => setAddAttr({ ...addAttr, type: e.target.value })}>
                {['number', 'string', 'boolean', 'json'].map((t) => (
                  <option key={t}>{t}</option>
                ))}
              </select>
            </div>
            <div className="col-md-1">
              <input className="form-control form-control-sm" placeholder="unit" value={addAttr.unit} onChange={(e) => setAddAttr({ ...addAttr, unit: e.target.value })} />
            </div>
            <div className="col-md-1 d-flex align-items-center">
              <input type="checkbox" checked={addAttr.writable} onChange={(e) => setAddAttr({ ...addAttr, writable: e.target.checked })} />
            </div>
            <div className="col-md-2">
              <button
                type="button"
                className="btn btn-outline-secondary btn-sm w-100"
                disabled={busy}
                onClick={() => {
                  const key = addAttr.key.trim();
                  if (!key) return alert('Attribute key required');
                  if (attributes.some((a) => a.key === key)) return alert('Attribute already exists');
                  const next = [...attributes, { ...addAttr, key, label: addAttr.label || key }];
                  patchDraft({ attributes: next });
                  setAddAttr({ key: '', label: '', scope: 'desired', type: 'number', unit: '', writable: true });
                  void persist({ attributes: next }, { quiet: true }).then(() => setStatus(`Attribute "${key}" saved`));
                }}
              >
                Add & save
              </button>
            </div>
          </div>
        </details>
      </div>
    </div>
  );
}
