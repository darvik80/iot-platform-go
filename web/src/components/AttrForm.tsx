import { useMemo, useState } from 'react';
import type { Product, ProductAttribute } from '../types';
import { propLabel } from '../utils/format';

function inferType(schemaType: string | undefined, value: unknown) {
  if (schemaType) return schemaType;
  if (typeof value === 'boolean') return 'boolean';
  if (typeof value === 'number') return 'number';
  if (typeof value === 'object' && value !== null) return 'json';
  return 'string';
}

function valStr(v: unknown) {
  if (v === undefined || v === null) return '';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

export function AttrForm({
  product,
  reported = {},
  desired = {},
  onSave,
}: {
  product?: Product;
  reported?: Record<string, unknown>;
  desired?: Record<string, unknown>;
  onSave: (attrs: Record<string, unknown>) => Promise<void>;
}) {
  const schema = product?.attributes || [];
  const [draft, setDraft] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    Object.keys(desired).forEach((k) => {
      init[k] = valStr(desired[k]);
    });
    return init;
  });
  const [addKey, setAddKey] = useState('');
  const [addVal, setAddVal] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const rows = useMemo(() => {
    if (schema.length) {
      return schema.map((a: ProductAttribute) => ({
        key: a.key,
        label: a.label || a.key,
        type: a.type || 'string',
        unit: a.unit || '',
        writable: !!(a.writable || a.scope === 'desired' || a.scope === 'both'),
      }));
    }
    const keys = new Set([...Object.keys(reported), ...Object.keys(desired)]);
    return [...keys].sort().map((key) => {
      const sample = reported[key] !== undefined ? reported[key] : desired[key];
      return {
        key,
        label: key,
        type: inferType('', sample),
        unit: '',
        writable: true,
      };
    });
  }, [schema, reported, desired]);

  const save = async () => {
    setBusy(true);
    setErr('');
    try {
      const attrs: Record<string, unknown> = {};
      rows.forEach((r) => {
        if (!r.writable) return;
        const v = draft[r.key];
        if (v === undefined || v === '') return;
        if (r.type === 'boolean') attrs[r.key] = v === 'true';
        else if (r.type === 'number') {
          const n = Number(v);
          if (!Number.isNaN(n)) attrs[r.key] = n;
        } else if (r.type === 'json') {
          try {
            attrs[r.key] = JSON.parse(v);
          } catch {
            attrs[r.key] = v;
          }
        } else attrs[r.key] = v;
      });
      if (addKey.trim()) {
        let v: unknown = addVal;
        if (v === 'true') v = true;
        else if (v === 'false') v = false;
        else if (v !== '' && !Number.isNaN(Number(v))) v = Number(v);
        attrs[addKey.trim()] = v;
      }
      if (!Object.keys(attrs).length) throw new Error('Nothing to save');
      await onSave(attrs);
      setAddKey('');
      setAddVal('');
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel">
      <h2 className="h6 text-uppercase muted">Attributes</h2>
      <p className="muted small">Reported from device · Desired editable and pushed on save</p>
      {!rows.length ? (
        <p className="muted">No attributes yet. Define them on the product schema, or wait for the device to report.</p>
      ) : (
        <div className="table-responsive">
          <table className="table table-sm align-middle">
            <thead>
              <tr>
                <th>Property</th>
                <th>Reported</th>
                <th>Desired</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => {
                const rv = reported[a.key];
                const type = inferType(a.type, rv !== undefined ? rv : desired[a.key]);
                const pl = propLabel(a.label, a.key);
                return (
                  <tr key={a.key}>
                    <td>
                      <div className="prop-cell">
                        <span className={pl.secondary ? '' : 'mono'}>{pl.primary}</span>
                        {pl.secondary ? <div className="m-key">{pl.secondary}</div> : null}
                      </div>
                    </td>
                    <td>
                      {rv === undefined || rv === null ? (
                        <span className="muted">—</span>
                      ) : type === 'boolean' ? (
                        <span className={`badge ${rv ? 'badge-online' : 'badge-info'}`}>{String(!!rv)}</span>
                      ) : (
                        <input className="form-control form-control-sm mono" readOnly value={valStr(rv)} />
                      )}
                      {a.unit ? <span className="muted small ms-1">{a.unit}</span> : null}
                    </td>
                    <td>
                      {!a.writable ? (
                        <span className="muted">—</span>
                      ) : type === 'boolean' ? (
                        <select
                          className="form-select form-select-sm"
                          value={draft[a.key] ?? valStr(desired[a.key])}
                          onChange={(e) => setDraft((d) => ({ ...d, [a.key]: e.target.value }))}
                        >
                          <option value="">(unset)</option>
                          <option value="true">true</option>
                          <option value="false">false</option>
                        </select>
                      ) : (
                        <input
                          className="form-control form-control-sm"
                          type={type === 'number' ? 'number' : 'text'}
                          value={draft[a.key] ?? valStr(desired[a.key])}
                          onChange={(e) => setDraft((d) => ({ ...d, [a.key]: e.target.value }))}
                        />
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {!schema.length ? (
        <div className="row g-2 mb-2">
          <div className="col-md-4">
            <input className="form-control form-control-sm mono" placeholder="newKey" value={addKey} onChange={(e) => setAddKey(e.target.value)} />
          </div>
          <div className="col-md-4">
            <input className="form-control form-control-sm" placeholder="value" value={addVal} onChange={(e) => setAddVal(e.target.value)} />
          </div>
        </div>
      ) : null}
      {err ? <div className="text-danger small mb-2">{err}</div> : null}
      <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={save}>
        Save desired & push
      </button>
    </div>
  );
}
