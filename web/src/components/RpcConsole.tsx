import { useEffect, useRef, useState } from 'react';
import type { Product, ProductMethod, ProductMethodParam } from '../types';
import { api } from '../api/client';

type RpcParamSpec = {
  type?: string;
  required?: boolean;
  default?: unknown;
};

type RpcListEntry = {
  method?: string;
  description?: string;
  params?: Record<string, string | RpcParamSpec>;
  required?: string[];
};

type RPCCall = {
  status?: string;
  code?: number | null;
  message?: string;
  data_json?: string | null;
};

function mapRpcType(t: string): ProductMethodParam['type'] {
  const x = (t || 'string').toLowerCase();
  if (x === 'int' || x === 'integer' || x === 'number' || x === 'float' || x === 'double') return 'number';
  if (x === 'bool' || x === 'boolean') return 'boolean';
  return 'string';
}

function isRpcListMethod(name: string) {
  const n = (name || '').toLowerCase();
  return n === 'rpc-list' || n === 'rpc_list';
}

function parseParamSpec(raw: string | RpcParamSpec | undefined): RpcParamSpec {
  if (typeof raw === 'string') return { type: raw };
  if (raw && typeof raw === 'object') {
    return {
      type: typeof raw.type === 'string' ? raw.type : 'string',
      required: !!raw.required,
      default: raw.default,
    };
  }
  return { type: 'string' };
}

function catalogToMethods(entries: RpcListEntry[], fallback: ProductMethod[]): ProductMethod[] {
  const byName = Object.fromEntries(fallback.map((m) => [m.name, m]));
  return entries
    .filter((e) => e.method)
    .map((e) => {
      const name = e.method as string;
      const prev = byName[name];
      const requiredNames = new Set(e.required || []);
      const params: ProductMethodParam[] = Object.entries(e.params || {}).map(([pname, specRaw]) => {
        const spec = parseParamSpec(specRaw);
        const old = prev?.params?.find((p) => p.name === pname);
        const required = spec.required === true || requiredNames.has(pname) || !!old?.required;
        const def = spec.default !== undefined ? spec.default : old?.default;
        return {
          name: pname,
          label: old?.label || pname,
          type: mapRpcType(spec.type || 'string'),
          default: def,
          required,
        };
      });
      return {
        name,
        label: prev?.label && prev.label !== name ? prev.label : '',
        description: e.description || prev?.description || '',
        params,
      };
    });
}

function methodCaption(m: ProductMethod) {
  const label = (m.label || '').trim();
  const desc = (m.description || '').trim();
  if (label && label !== m.name) return label;
  if (desc) return desc;
  return m.name;
}

function methodOptionLabel(m: ProductMethod) {
  const cap = methodCaption(m);
  return cap !== m.name ? `${cap} (${m.name})` : m.name;
}

function parseRpcList(call: RPCCall): RpcListEntry[] | null {
  if (!call.data_json) return null;
  try {
    const data = JSON.parse(call.data_json);
    if (!Array.isArray(data)) return null;
    return data as RpcListEntry[];
  } catch {
    return null;
  }
}

export function RpcConsole({
  product,
  deviceId,
  online,
}: {
  product?: Product;
  deviceId: number;
  online?: boolean;
}) {
  const fallback = product?.methods || [];
  const fallbackRef = useRef(fallback);
  fallbackRef.current = fallback;
  const [live, setLive] = useState<ProductMethod[] | null>(null);
  const [catalogNote, setCatalogNote] = useState('');
  const [idx, setIdx] = useState(0);
  const [params, setParams] = useState<Record<string, string>>({});
  const [result, setResult] = useState('Result will appear here');
  const [busy, setBusy] = useState(false);

  const methods = live && live.length ? live : fallback;
  const method: ProductMethod | undefined = methods[idx] || methods[0];

  const applyCatalog = (call: RPCCall) => {
    const entries = parseRpcList(call);
    if (!entries) return false;
    const next = catalogToMethods(entries, fallbackRef.current);
    if (!next.length) return false;
    setLive(next);
    setIdx((cur) => (cur < next.length ? cur : 0));
    setCatalogNote(`Firmware catalog: ${next.length} methods`);
    return true;
  };

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
        if (!p.required && raw === '') return;
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
      const res = await api<RPCCall>(`/devices/${deviceId}/rpc`, {
        method: 'POST',
        body: JSON.stringify({ method: target.name, params: bodyParams }),
      });
      if (isRpcListMethod(target.name)) applyCatalog(res);
      setResult(JSON.stringify(res, null, 2));
    } catch (e) {
      setResult(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    if (!online) {
      setCatalogNote('');
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await api<RPCCall>(`/devices/${deviceId}/rpc`, {
          method: 'POST',
          body: JSON.stringify({ method: 'rpc-list', params: {} }),
        });
        if (cancelled) return;
        if (!applyCatalog(res)) setCatalogNote('rpc-list returned no catalog; using product schema');
      } catch (e) {
        if (!cancelled) setCatalogNote(e instanceof Error ? e.message : 'rpc-list failed; using product schema');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [deviceId, online]);

  if (!methods.length) {
    return <p className="muted mb-0">Register methods on the product to call them here.</p>;
  }

  return (
    <div>
      {catalogNote && <p className="small muted mb-2">{catalogNote}</p>}
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
            title={m.description || m.name}
          >
            {methodCaption(m)}
          </button>
        ))}
      </div>
      <div className="row g-2 mb-2">
        <div className="col-md-4">
          <label className="form-label small muted">Method</label>
          <select className="form-select form-select-sm" value={Math.min(idx, methods.length - 1)} onChange={(e) => setIdx(Number(e.target.value))}>
            {methods.map((m, i) => (
              <option key={m.name} value={i}>
                {methodOptionLabel(m)}
              </option>
            ))}
          </select>
        </div>
        {(method?.params || []).map((p) => (
          <div className="col-md-3" key={p.name}>
            <label className="form-label small muted">
              {p.label || p.name}
              {p.required ? ' *' : ''}
            </label>
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
      {method?.description && <p className="small muted mb-2">{method.description}</p>}
      <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => invoke()}>
        Invoke
      </button>
      <pre className="mt-3 small muted mb-0">{result}</pre>
    </div>
  );
}
