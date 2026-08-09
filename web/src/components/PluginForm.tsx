import type { PluginFormField, PluginFormOption } from '../types';

const TZ_OPTIONS = [
  'UTC',
  'Asia/Bangkok',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Asia/Shanghai',
  'Asia/Almaty',
  'Europe/Moscow',
  'Europe/Kyiv',
  'Europe/Berlin',
  'Europe/London',
  'America/New_York',
  'America/Los_Angeles',
];

const WEEKDAYS = [
  { d: 0, label: 'Su' },
  { d: 1, label: 'Mo' },
  { d: 2, label: 'Tu' },
  { d: 3, label: 'We' },
  { d: 4, label: 'Th' },
  { d: 5, label: 'Fr' },
  { d: 6, label: 'Sa' },
];

const SCALAR = new Set(['number', 'string', 'select', 'time', 'timezone', 'checkbox']);

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? { ...(v as Record<string, unknown>) } : {};
}

function asList(v: unknown): unknown[] {
  return Array.isArray(v) ? [...v] : [];
}

function selectOptions(field: PluginFormField, effects?: string[]): PluginFormOption[] {
  if (field.options_from === 'effects') {
    const labels = new Map((field.options || []).map((o) => [o.value, o.label]));
    return (effects?.length ? effects : field.options?.map((o) => o.value) || []).map((v) => ({
      value: v,
      label: labels.get(v) || v,
    }));
  }
  return field.options || [];
}

function disabled(field: PluginFormField, root: Record<string, unknown>): boolean {
  if (!field.disabled_when) return false;
  return Object.entries(field.disabled_when).every(([k, v]) => root[k] === v);
}

function nextSuggest(existing: unknown[], field: PluginFormField): number | undefined {
  if (!field.suggest?.length) return undefined;
  const used = new Set(existing.map((row) => Number(asRecord(row)[field.key])));
  for (const n of field.suggest) {
    if (!used.has(n)) return n;
  }
  const last = existing.length ? Number(asRecord(existing[existing.length - 1])[field.key]) : 0;
  return (Number.isFinite(last) ? last : 0) + 1;
}

function newItem(field: PluginFormField, existing: unknown[]): Record<string, unknown> {
  const item = asRecord(field.add);
  for (const f of field.fields || []) {
    if (f.type === 'number' && f.suggest?.length) {
      const n = nextSuggest(existing, f);
      if (n !== undefined) item[f.key] = n;
    }
  }
  return item;
}

export function PluginForm({
  fields,
  value,
  onChange,
  root,
  effects,
  platformTz,
  idPrefix,
}: {
  fields: PluginFormField[];
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  root: Record<string, unknown>;
  effects?: string[];
  platformTz: string;
  idPrefix: string;
}) {
  const set = (key: string, v: unknown) => onChange({ ...value, [key]: v });

  return (
    <div className="row g-2 align-items-end">
      {fields.map((field) => {
        if (field.type === 'array') {
          return (
            <div key={field.key} className="col-12 mb-1">
              <ArrayField field={field} value={asList(value[field.key])} onChange={(v) => set(field.key, v)} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${field.key}`} />
            </div>
          );
        }
        if (field.type === 'object') {
          return (
            <div key={field.key} className={field.wide ? 'col-12' : 'col-md-6'}>
              <ObjectField field={field} value={asRecord(value[field.key])} onChange={(v) => set(field.key, v)} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${field.key}`} />
            </div>
          );
        }
        if (field.type === 'weekdays' || field.type === 'json' || field.wide) {
          return (
            <div key={field.key} className="col-12">
              <ScalarField field={field} value={value[field.key]} onChange={(v) => set(field.key, v)} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${field.key}`} />
            </div>
          );
        }
        return (
          <div key={field.key} className="col-md-3">
            <ScalarField field={field} value={value[field.key]} onChange={(v) => set(field.key, v)} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${field.key}`} />
          </div>
        );
      })}
    </div>
  );
}

function ArrayField({
  field,
  value,
  onChange,
  root,
  effects,
  platformTz,
  idPrefix,
}: {
  field: PluginFormField;
  value: unknown[];
  onChange: (v: unknown[]) => void;
  root: Record<string, unknown>;
  effects?: string[];
  platformTz: string;
  idPrefix: string;
}) {
  const itemFields = field.fields || [];
  const tabular = itemFields.length > 0 && itemFields.every((f) => SCALAR.has(f.type));

  return (
    <>
      <div className="d-flex align-items-center gap-2 mb-1">
        {field.label && <span className="small text-uppercase muted">{field.label}</span>}
        <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => onChange([...value, newItem(field, value)])}>
          {field.add_label || 'Add'}
        </button>
      </div>
      {tabular ? (
        <div className="table-responsive mb-2">
          <table className="table table-sm mb-0 align-middle">
            <thead>
              <tr>
                {itemFields.map((f) => (
                  <th key={f.key}>{f.label || f.key}</th>
                ))}
                <th />
              </tr>
            </thead>
            <tbody>
              {value.map((row, i) => (
                <tr key={i}>
                  {itemFields.map((f) => (
                    <td key={f.key} style={{ width: f.type === 'number' ? 110 : undefined }}>
                      <ScalarField field={{ ...f, label: undefined }} value={asRecord(row)[f.key]} onChange={(v) => {
                        const next = value.map((r, j) => (j === i ? { ...asRecord(r), [f.key]: v } : r));
                        onChange(next);
                      }} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${i}-${f.key}`} />
                    </td>
                  ))}
                  <td>
                    <button type="button" className="btn btn-outline-danger btn-sm" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                      {field.remove_label || 'Delete'}
                    </button>
                  </td>
                </tr>
              ))}
              {!value.length && (
                <tr>
                  <td colSpan={itemFields.length + 1} className="muted small">
                    Empty
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      ) : (
        <>
          {value.map((row, i) => (
            <div key={i} className="border border-secondary rounded p-2 mb-2">
              <PluginForm
                fields={itemFields}
                value={asRecord(row)}
                onChange={(v) => onChange(value.map((r, j) => (j === i ? v : r)))}
                root={root}
                effects={effects}
                platformTz={platformTz}
                idPrefix={`${idPrefix}-${i}`}
              />
              <button type="button" className="btn btn-outline-danger btn-sm mt-2" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                {field.remove_label || 'Delete'}
              </button>
            </div>
          ))}
        </>
      )}
    </>
  );
}

function ObjectField({
  field,
  value,
  onChange,
  root,
  effects,
  platformTz,
  idPrefix,
}: {
  field: PluginFormField;
  value: Record<string, unknown>;
  onChange: (v: Record<string, unknown>) => void;
  root: Record<string, unknown>;
  effects?: string[];
  platformTz: string;
  idPrefix: string;
}) {
  const enabledField = (field.fields || []).find((f) => f.key === 'enabled' && f.type === 'checkbox');
  const rest = (field.fields || []).filter((f) => f !== enabledField);
  return (
    <div className="mb-1">
      {enabledField ? (
        <div className="form-check mb-1">
          <input className="form-check-input" type="checkbox" checked={!!value.enabled} onChange={(e) => onChange({ ...value, enabled: e.target.checked })} />
          <label className="form-check-label small">{field.label || enabledField.label}</label>
        </div>
      ) : (
        field.label && <div className="small text-uppercase muted mb-1">{field.label}</div>
      )}
      <div className="d-flex flex-wrap gap-2">
        {rest.map((f) => (
          <div key={f.key} style={{ minWidth: f.type === 'select' ? 120 : 110, flex: '1 1 90px' }}>
            <ScalarField field={f} value={value[f.key]} onChange={(v) => onChange({ ...value, [f.key]: v })} root={root} effects={effects} platformTz={platformTz} idPrefix={`${idPrefix}-${f.key}`} />
          </div>
        ))}
      </div>
    </div>
  );
}

function ScalarField({
  field,
  value,
  onChange,
  root,
  effects,
  platformTz,
  idPrefix,
}: {
  field: PluginFormField;
  value: unknown;
  onChange: (v: unknown) => void;
  root: Record<string, unknown>;
  effects?: string[];
  platformTz: string;
  idPrefix: string;
}) {
  const off = disabled(field, root);
  if (field.type === 'checkbox') {
    return (
      <div className="form-check">
        <input className="form-check-input" type="checkbox" checked={!!value} disabled={off} onChange={(e) => onChange(e.target.checked)} />
        {field.label && <label className="form-check-label small">{field.label}</label>}
      </div>
    );
  }
  if (field.type === 'weekdays') {
    const days = Array.isArray(value) ? (value as number[]) : [];
    return (
      <div>
        {field.label && <div className="form-label small mb-1">{field.label}</div>}
        <div className="d-flex flex-wrap gap-2">
          {WEEKDAYS.map((w) => {
            const on = !days.length || days.includes(w.d);
            return (
              <label key={w.d} className="form-check form-check-inline mb-0">
                <input
                  className="form-check-input"
                  type="checkbox"
                  checked={on}
                  disabled={off}
                  onChange={() => {
                    const cur = days.length ? [...days] : WEEKDAYS.map((x) => x.d);
                    const next = cur.includes(w.d) ? cur.filter((d) => d !== w.d) : [...cur, w.d].sort();
                    onChange(next.length === 7 ? [] : next);
                  }}
                />
                <span className="form-check-label small">{w.label}</span>
              </label>
            );
          })}
          {field.help && <span className="small muted">{field.help}</span>}
        </div>
      </div>
    );
  }
  if (field.type === 'json') {
    return (
      <div>
        {field.label && <label className="form-label small mb-1">{field.label}</label>}
        <textarea
          className="form-control form-control-sm mono"
          rows={2}
          disabled={off}
          defaultValue={JSON.stringify(value && typeof value === 'object' ? value : {})}
          key={JSON.stringify(value)}
          onBlur={(e) => {
            try {
              onChange(JSON.parse(e.target.value || '{}'));
            } catch {
              alert('Invalid JSON');
            }
          }}
        />
      </div>
    );
  }
  if (field.type === 'timezone') {
    return (
      <div>
        {field.label && <label className="form-label small mb-1">{field.label}</label>}
        <input
          className="form-control form-control-sm"
          list={`${idPrefix}-tz`}
          placeholder={platformTz || 'UTC'}
          disabled={off}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
        />
        <datalist id={`${idPrefix}-tz`}>
          {TZ_OPTIONS.map((z) => (
            <option key={z} value={z} />
          ))}
        </datalist>
      </div>
    );
  }
  if (field.type === 'select') {
    const opts = selectOptions(field, effects);
    return (
      <div>
        {field.label && <label className="form-label small mb-1">{field.label}</label>}
        <select className="form-select form-select-sm" disabled={off} value={String(value ?? '')} onChange={(e) => onChange(e.target.value)}>
          {opts.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>
    );
  }
  if (field.type === 'time') {
    return (
      <div>
        {field.label && <label className="form-label small mb-1">{field.label}</label>}
        <input className="form-control form-control-sm" type="time" disabled={off} value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value)} />
      </div>
    );
  }
  if (field.type === 'number') {
    return (
      <div>
        {field.label && <label className="form-label small mb-1">{field.label}</label>}
        <input
          className="form-control form-control-sm"
          type="number"
          disabled={off}
          min={field.min}
          max={field.max}
          value={typeof value === 'number' ? value : Number(value) || 0}
          onChange={(e) => onChange(Number(e.target.value))}
        />
      </div>
    );
  }
  return (
    <div>
      {field.label && <label className="form-label small mb-1">{field.label}</label>}
      <input className="form-control form-control-sm" disabled={off} value={value == null ? '' : String(value)} onChange={(e) => onChange(e.target.value)} placeholder={field.placeholder} />
    </div>
  );
}
