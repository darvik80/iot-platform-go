import type { ProductMethod, ProductMethodParam } from '../types';

export const PRESET_METHODS: Record<string, boolean> = { reboot: true };
export const PRESET_METRICS: Record<string, true> = {
  cpuUsagePercent: true,
  uptimeSeconds: true,
  wifiConnected: true,
  wifiIp: true,
  wifiRssi: true,
  freeHeap: true,
  freeDram: true,
  minFreeHeap: true,
  largestFreeBlock: true,
  freePsram: true,
  minFreePsram: true,
  storageTotalBytes: true,
  storageUsedBytes: true,
};

function ParamRows({
  params,
  onChange,
}: {
  params: ProductMethodParam[];
  onChange: (params: ProductMethodParam[]) => void;
}) {
  return (
    <div className="table-responsive">
      <table className="table table-sm schema-compact mb-1">
        <thead>
          <tr>
            <th>Param</th>
            <th>Label</th>
            <th>Type</th>
            <th>Default</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {params.length ? (
            params.map((p, i) => (
              <tr key={i}>
                <td>
                  <input
                    className="form-control form-control-sm mono"
                    value={p.name || ''}
                    onChange={(e) => {
                      const next = [...params];
                      next[i] = { ...p, name: e.target.value };
                      onChange(next);
                    }}
                  />
                </td>
                <td>
                  <input
                    className="form-control form-control-sm"
                    value={p.label || ''}
                    onChange={(e) => {
                      const next = [...params];
                      next[i] = { ...p, label: e.target.value };
                      onChange(next);
                    }}
                  />
                </td>
                <td>
                  <select
                    className="form-select form-select-sm"
                    value={p.type || 'number'}
                    onChange={(e) => {
                      const next = [...params];
                      next[i] = { ...p, type: e.target.value };
                      onChange(next);
                    }}
                  >
                    {['number', 'string', 'boolean', 'json'].map((t) => (
                      <option key={t} value={t}>
                        {t}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <input
                    className="form-control form-control-sm"
                    value={p.default === undefined || p.default === null ? '' : String(p.default)}
                    onChange={(e) => {
                      const next = [...params];
                      let def: unknown = e.target.value;
                      if ((p.type || 'number') === 'number' && def !== '') def = Number(def);
                      if (p.type === 'boolean') def = def === 'true';
                      next[i] = { ...p, default: def === '' ? undefined : def };
                      onChange(next);
                    }}
                  />
                </td>
                <td>
                  <button
                    type="button"
                    className="btn btn-outline-secondary btn-sm"
                    onClick={() => onChange(params.filter((_, j) => j !== i))}
                  >
                    ×
                  </button>
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td colSpan={5} className="muted small">
                No parameters
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <button
        type="button"
        className="btn btn-outline-secondary btn-sm"
        onClick={() => onChange([...params, { name: '', type: 'number' }])}
      >
        + Param
      </button>
    </div>
  );
}

export function MethodEditor({
  methods,
  onChange,
}: {
  methods: ProductMethod[];
  onChange: (m: ProductMethod[]) => void;
}) {
  return (
    <div>
      {methods.map((m, i) => {
        const preset = !!PRESET_METHODS[m.name];
        return (
          <div className="method-block" key={`${m.name}-${i}`}>
            <div className="row g-2 align-items-end mb-2">
              <div className="col-md-3">
                <label className="form-label small muted mb-0">Name</label>
                <input
                  className="form-control form-control-sm mono"
                  value={m.name}
                  readOnly={preset}
                  onChange={(e) => {
                    const next = [...methods];
                    next[i] = { ...m, name: e.target.value };
                    onChange(next);
                  }}
                />
              </div>
              <div className="col-md-3">
                <label className="form-label small muted mb-0">Label</label>
                <input
                  className="form-control form-control-sm"
                  value={m.label || ''}
                  onChange={(e) => {
                    const next = [...methods];
                    next[i] = { ...m, label: e.target.value };
                    onChange(next);
                  }}
                />
              </div>
              <div className="col-md-4">
                <label className="form-label small muted mb-0">Description</label>
                <input
                  className="form-control form-control-sm"
                  value={m.description || ''}
                  onChange={(e) => {
                    const next = [...methods];
                    next[i] = { ...m, description: e.target.value };
                    onChange(next);
                  }}
                />
              </div>
              <div className="col-md-2">
                {preset ? (
                  <span className="badge badge-info">preset</span>
                ) : (
                  <button
                    type="button"
                    className="btn btn-outline-secondary btn-sm"
                    onClick={() => onChange(methods.filter((_, j) => j !== i))}
                  >
                    Remove
                  </button>
                )}
              </div>
            </div>
            <ParamRows
              params={m.params || []}
              onChange={(params) => {
                const next = [...methods];
                next[i] = { ...m, params };
                onChange(next);
              }}
            />
          </div>
        );
      })}
    </div>
  );
}
