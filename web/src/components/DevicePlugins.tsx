import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { useAuth } from '../auth/AuthContext';
import type { DevicePluginBinding, PluginFormField, PluginInfo } from '../types';
import { PluginForm } from './PluginForm';

function asConfig(v: unknown): Record<string, unknown> {
  if (v && typeof v === 'object' && !Array.isArray(v)) return { ...(v as Record<string, unknown>) };
  return {};
}

function parseForm(raw: unknown): PluginFormField[] {
  return Array.isArray(raw) ? (raw as PluginFormField[]) : [];
}

export function DevicePlugins({ deviceId, online }: { deviceId: number; online: boolean }) {
  const platformTz = useAuth().settings?.timezone || 'UTC';
  const [catalog, setCatalog] = useState<PluginInfo[]>([]);
  const [bindings, setBindings] = useState<DevicePluginBinding[]>([]);
  const [addId, setAddId] = useState('');
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [cat, list] = await Promise.all([
        api<PluginInfo[]>('/plugins'),
        api<DevicePluginBinding[]>(`/devices/${deviceId}/plugins`),
      ]);
      setCatalog(cat);
      setBindings(list);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [deviceId]);

  useEffect(() => {
    void load();
    return platformWS.subscribe((msg) => {
      if (msg.event === 'device.status') void load();
    });
  }, [load]);

  const bound = useMemo(() => new Set(bindings.map((b) => b.plugin_id)), [bindings]);
  const available = catalog.filter((p) => !bound.has(p.id));

  const add = async () => {
    if (!addId) return;
    setBusy('add');
    try {
      await api(`/devices/${deviceId}/plugins`, { method: 'POST', body: JSON.stringify({ plugin_id: addId }) });
      setAddId('');
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="panel">
      <h2 className="h6 text-uppercase muted">Plugins</h2>
      <p className="small muted mb-2">
        Device-bound automations. Form schema comes from the plugin. Times use platform TZ <span className="mono">{platformTz}</span> unless a plugin sets its own.
      </p>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      {bindings.map((b) => (
        <BindingCard
          key={b.id}
          deviceId={deviceId}
          binding={b}
          catalogForm={parseForm(catalog.find((p) => p.id === b.plugin_id)?.form)}
          online={online}
          platformTz={platformTz}
          busy={busy}
          setBusy={setBusy}
          onChanged={load}
        />
      ))}
      {!bindings.length && <p className="muted small mb-2">No plugins bound.</p>}
      {!!available.length && (
        <div className="d-flex flex-wrap gap-2 align-items-center mt-2">
          <select className="form-select form-select-sm" style={{ maxWidth: 260 }} value={addId} onChange={(e) => setAddId(e.target.value)}>
            <option value="">Add plugin…</option>
            {available.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button type="button" className="btn btn-outline-secondary btn-sm" disabled={!!busy || !addId} onClick={() => void add()}>
            Add
          </button>
        </div>
      )}
    </div>
  );
}

function BindingCard({
  deviceId,
  binding,
  catalogForm,
  online,
  platformTz,
  busy,
  setBusy,
  onChanged,
}: {
  deviceId: number;
  binding: DevicePluginBinding;
  catalogForm: PluginFormField[];
  online: boolean;
  platformTz: string;
  busy: string;
  setBusy: (v: string) => void;
  onChanged: () => Promise<void>;
}) {
  const [cfg, setCfg] = useState<Record<string, unknown>>(() => asConfig(binding.config));
  useEffect(() => {
    setCfg(asConfig(binding.config));
  }, [binding.id, binding.updated_at]);

  const form = parseForm(binding.form).length ? parseForm(binding.form) : catalogForm;
  const key = `p-${binding.id}`;
  const interval = typeof cfg.interval_ms === 'number' ? cfg.interval_ms : undefined;

  const save = async (next: Record<string, unknown>, enabled = binding.enabled) => {
    setBusy(key);
    try {
      await api(`/devices/${deviceId}/plugins/${binding.id}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled, config: next }),
      });
      await onChanged();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const apply = async () => {
    setBusy(key);
    try {
      await api(`/devices/${deviceId}/plugins/${binding.id}/apply`, { method: 'POST', body: '{}' });
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const remove = async () => {
    if (!confirm(`Remove plugin ${binding.name || binding.plugin_id}?`)) return;
    setBusy(key);
    try {
      await api(`/devices/${deviceId}/plugins/${binding.id}`, { method: 'DELETE' });
      await onChanged();
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="border border-secondary rounded p-2 mb-2">
      <div className="d-flex flex-wrap align-items-center gap-2 mb-2">
        <strong>{binding.name || binding.plugin_id}</strong>
        <span className="small muted">{binding.description}</span>
        <span className={`badge ${binding.running ? 'text-bg-success' : 'text-bg-secondary'}`}>
          {binding.running ? `running${interval != null ? ` ${interval}ms` : ''}` : binding.enabled ? (online ? 'idle' : 'offline') : 'disabled'}
        </span>
        <div className="form-check form-switch mb-0 ms-auto">
          <input className="form-check-input" type="checkbox" checked={binding.enabled} disabled={!!busy} onChange={(e) => void save(cfg, e.target.checked)} />
          <label className="form-check-label small">enabled</label>
        </div>
      </div>

      {form.length ? (
        <PluginForm fields={form} value={cfg} onChange={setCfg} root={cfg} effects={binding.effects} platformTz={platformTz} idPrefix={key} />
      ) : (
        <div className="small muted mb-2">No configuration required.</div>
      )}

      <div className="d-flex flex-wrap gap-2 mt-2">
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={!!busy}
          onClick={() => {
            void save(cfg);
          }}
        >
          Save
        </button>
        <button type="button" className="btn btn-outline-secondary btn-sm" disabled={!!busy || !online || !binding.enabled} onClick={() => void apply()}>
          Apply now
        </button>
        <button type="button" className="btn btn-outline-danger btn-sm" disabled={!!busy} onClick={() => void remove()}>
          Remove plugin
        </button>
      </div>
      {binding.last_error && <div className="small text-danger mt-2">{binding.last_error}</div>}
      {!online && <div className="small muted mt-1">Device offline — auto paused, Apply needs online.</div>}
    </div>
  );
}
