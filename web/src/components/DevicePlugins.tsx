import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import { platformWS } from '../api/ws';
import { useAuth } from '../auth/AuthContext';
import type {
  DevicePluginBinding,
  LedGarlandConfig,
  LedGarlandStrip,
  LedTimeWindow,
  PluginInfo,
  ScheduleConfig,
  ScheduleJob,
} from '../types';

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

const EFFECT_LABELS: Record<string, string> = {
  off: 'Off',
  white: 'Solid white',
  warm: 'Warm white',
  red: 'Solid red',
  green: 'Solid green',
  blue: 'Solid blue',
  blink: 'Blink',
  strobe: 'Strobe',
  breathe: 'Breathe',
  chase: 'Chase',
  comet: 'Comet',
  scanner: 'Scanner (cylon)',
  fill: 'Fill',
  theater: 'Theater chase',
  sparkle: 'Sparkle',
  fire: 'Fire',
  rainbow: 'Rainbow wash',
  police: 'Police',
  christmas: 'Christmas',
};

function asGarland(cfg: LedGarlandConfig | undefined): LedGarlandConfig {
  const strips = Array.isArray(cfg?.strips)
    ? cfg.strips.map((s) => ({ gpio: Number(s.gpio) || 0, length: Math.max(1, Number(s.length) || 1) }))
    : [
        { gpio: 16, length: 8 },
        { gpio: 17, length: 8 },
      ];
  const win = (w: LedTimeWindow | undefined, from: string, to: string, effect?: string): LedTimeWindow => ({
    enabled: !!w?.enabled,
    from: w?.from || from,
    to: w?.to || to,
    ...(effect !== undefined ? { effect: w?.effect || effect } : {}),
  });
  return {
    strips,
    mode: cfg?.mode === 'auto' ? 'auto' : 'effect',
    effect: cfg?.effect || 'off',
    interval_ms: cfg?.interval_ms || 200,
    auto_ms: cfg?.auto_ms || 8000,
    rpc: cfg?.rpc || 'set_led',
    attach_rpc: cfg?.attach_rpc || 'led_attach',
    brightness: cfg?.brightness || 32,
    tz: cfg?.tz || '',
    quiet: win(cfg?.quiet, '23:00', '07:00', 'off'),
    active: win(cfg?.active, '18:00', '23:00'),
  };
}

function asSchedule(cfg: ScheduleConfig | undefined): ScheduleConfig {
  const jobs = Array.isArray(cfg?.jobs)
    ? cfg.jobs.map((j) => ({
        name: j.name || '',
        at: j.at || '23:00',
        days: Array.isArray(j.days) ? j.days : [],
        rpc: j.rpc || 'set_led',
        params: j.params && typeof j.params === 'object' ? j.params : {},
      }))
    : [];
  return { tz: cfg?.tz || '', jobs };
}

function nextGpio(strips: LedGarlandStrip[]): number {
  const used = new Set(strips.map((s) => s.gpio));
  for (const g of [16, 17, 18, 19, 21, 22, 23, 25, 26, 27, 32, 33]) {
    if (!used.has(g)) return g;
  }
  return (strips[strips.length - 1]?.gpio ?? 15) + 1;
}

function TzField({
  value,
  onChange,
  platformTz,
  listId,
}: {
  value: string;
  onChange: (v: string) => void;
  platformTz: string;
  listId: string;
}) {
  return (
    <>
      <input
        className="form-control form-control-sm"
        list={listId}
        placeholder={platformTz || 'UTC'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <datalist id={listId}>
        {TZ_OPTIONS.map((z) => (
          <option key={z} value={z} />
        ))}
      </datalist>
    </>
  );
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
        Device-bound automations. Same events as Node-RED, RPC back to this device. Times use platform TZ{' '}
        <span className="mono">{platformTz}</span> unless a plugin sets its own. Auto/schedule keep running while the device is online.
      </p>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      {bindings.map((b) =>
        b.plugin_id === 'led-garland' ? (
          <GarlandBinding
            key={b.id}
            deviceId={deviceId}
            binding={b}
            online={online}
            platformTz={platformTz}
            busy={busy}
            setBusy={setBusy}
            onChanged={load}
          />
        ) : b.plugin_id === 'schedule' ? (
          <ScheduleBinding
            key={b.id}
            deviceId={deviceId}
            binding={b}
            online={online}
            platformTz={platformTz}
            busy={busy}
            setBusy={setBusy}
            onChanged={load}
          />
        ) : (
          <GenericBinding
            key={b.id}
            binding={b}
            busy={!!busy}
            onRemove={async () => {
              if (!confirm(`Remove plugin ${b.name || b.plugin_id}?`)) return;
              setBusy(`p-${b.id}`);
              try {
                await api(`/devices/${deviceId}/plugins/${b.id}`, { method: 'DELETE' });
                await load();
              } catch (e) {
                alert(e instanceof Error ? e.message : String(e));
              } finally {
                setBusy('');
              }
            }}
            onSave={async (enabled, config) => {
              setBusy(`p-${b.id}`);
              try {
                await api(`/devices/${deviceId}/plugins/${b.id}`, {
                  method: 'PUT',
                  body: JSON.stringify({ enabled, config }),
                });
                await load();
              } catch (e) {
                alert(e instanceof Error ? e.message : String(e));
              } finally {
                setBusy('');
              }
            }}
          />
        ),
      )}
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

function GarlandBinding({
  deviceId,
  binding,
  online,
  platformTz,
  busy,
  setBusy,
  onChanged,
}: {
  deviceId: number;
  binding: DevicePluginBinding;
  online: boolean;
  platformTz: string;
  busy: string;
  setBusy: (v: string) => void;
  onChanged: () => Promise<void>;
}) {
  const [cfg, setCfg] = useState<LedGarlandConfig>(() => asGarland(binding.config as LedGarlandConfig));
  useEffect(() => {
    setCfg(asGarland(binding.config as LedGarlandConfig));
  }, [binding.id, binding.updated_at]);

  const effects = binding.effects?.length ? binding.effects : ['off', 'blink', 'chase', 'rainbow', 'breathe'];
  const key = `p-${binding.id}`;

  const save = async (next: LedGarlandConfig, enabled = binding.enabled) => {
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

  const setStrip = (i: number, patch: Partial<LedGarlandStrip>) => {
    const strips = [...(cfg.strips || [])];
    strips[i] = { ...strips[i], ...patch };
    setCfg({ ...cfg, strips });
  };

  return (
    <div className="border border-secondary rounded p-2 mb-2">
      <div className="d-flex flex-wrap align-items-center gap-2 mb-2">
        <strong>{binding.name || 'LED garland'}</strong>
        <span className="small muted">{binding.description}</span>
        <span className={`badge ${binding.running ? 'text-bg-success' : 'text-bg-secondary'}`}>
          {binding.running ? `running ${cfg.interval_ms}ms` : binding.enabled ? (online ? 'idle' : 'offline') : 'disabled'}
        </span>
        <div className="form-check form-switch mb-0 ms-auto">
          <input
            className="form-check-input"
            type="checkbox"
            checked={binding.enabled}
            disabled={!!busy}
            onChange={(e) => void save(cfg, e.target.checked)}
          />
          <label className="form-check-label small">enabled</label>
        </div>
      </div>

      <div className="d-flex align-items-center gap-2 mb-1">
        <span className="small text-uppercase muted">Strips</span>
        <button
          type="button"
          className="btn btn-outline-secondary btn-sm"
          disabled={!!busy}
          onClick={() => {
            const strips = [...(cfg.strips || []), { gpio: nextGpio(cfg.strips || []), length: 8 }];
            const next = { ...cfg, strips };
            setCfg(next);
            void save(next);
          }}
        >
          Add strip
        </button>
      </div>
      <div className="table-responsive mb-2">
        <table className="table table-sm mb-0 align-middle">
          <thead>
            <tr>
              <th>GPIO</th>
              <th>LEDs</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(cfg.strips || []).map((s, i) => (
              <tr key={`${s.gpio}-${i}`}>
                <td style={{ width: 110 }}>
                  <input
                    className="form-control form-control-sm"
                    type="number"
                    min={0}
                    value={s.gpio}
                    onChange={(e) => setStrip(i, { gpio: Number(e.target.value) })}
                  />
                </td>
                <td style={{ width: 110 }}>
                  <input
                    className="form-control form-control-sm"
                    type="number"
                    min={1}
                    value={s.length}
                    onChange={(e) => setStrip(i, { length: Number(e.target.value) })}
                  />
                </td>
                <td>
                  <button
                    type="button"
                    className="btn btn-outline-danger btn-sm"
                    disabled={!!busy}
                    onClick={() => {
                      const strips = (cfg.strips || []).filter((_, j) => j !== i);
                      const next = { ...cfg, strips };
                      setCfg(next);
                      void save(next);
                    }}
                  >
                    Delete strip
                  </button>
                </td>
              </tr>
            ))}
            {!(cfg.strips || []).length && (
              <tr>
                <td colSpan={3} className="muted small">
                  No strips — Add strip, then Save.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="row g-2 align-items-end mb-2">
        <div className="col-md-3">
          <label className="form-label small mb-1">Mode</label>
          <select className="form-select form-select-sm" value={cfg.mode} onChange={(e) => setCfg({ ...cfg, mode: e.target.value as 'auto' | 'effect' })}>
            <option value="effect">effect</option>
            <option value="auto">auto (cycle)</option>
          </select>
        </div>
        <div className="col-md-3">
          <label className="form-label small mb-1">Effect</label>
          <select
            className="form-select form-select-sm"
            value={cfg.effect}
            disabled={cfg.mode === 'auto'}
            onChange={(e) => setCfg({ ...cfg, effect: e.target.value })}
          >
            {effects.map((ef) => (
              <option key={ef} value={ef}>
                {EFFECT_LABELS[ef] || ef}
              </option>
            ))}
          </select>
        </div>
        <div className="col-md-2">
          <label className="form-label small mb-1">Frame interval ms</label>
          <input
            className="form-control form-control-sm"
            type="number"
            min={80}
            value={cfg.interval_ms}
            onChange={(e) => setCfg({ ...cfg, interval_ms: Number(e.target.value) })}
          />
        </div>
        <div className="col-md-2">
          <label className="form-label small mb-1">Brightness</label>
          <input
            className="form-control form-control-sm"
            type="number"
            min={1}
            max={255}
            value={cfg.brightness || 32}
            onChange={(e) => setCfg({ ...cfg, brightness: Number(e.target.value) })}
          />
        </div>
        <div className="col-md-3">
          <label className="form-label small mb-1">Timezone</label>
          <TzField listId={`tz-garland-${binding.id}`} value={cfg.tz || ''} onChange={(tz) => setCfg({ ...cfg, tz })} platformTz={platformTz} />
        </div>
        <div className="col-md-4 d-flex gap-2">
          <button type="button" className="btn btn-primary btn-sm" disabled={!!busy} onClick={() => void save(cfg)}>
            Save
          </button>
          <button type="button" className="btn btn-outline-secondary btn-sm" disabled={!!busy || !online || !binding.enabled} onClick={() => void apply()}>
            Apply now
          </button>
          <button type="button" className="btn btn-outline-danger btn-sm" disabled={!!busy} onClick={() => void remove()}>
            Remove plugin
          </button>
        </div>
      </div>
      <div className="row g-2 align-items-end mb-2">
        <div className="col-md-5">
          <div className="form-check mb-1">
            <input
              className="form-check-input"
              type="checkbox"
              checked={!!cfg.quiet?.enabled}
              onChange={(e) => setCfg({ ...cfg, quiet: { ...cfg.quiet, enabled: e.target.checked } })}
            />
            <label className="form-check-label small">Quiet hours (wraps midnight)</label>
          </div>
          <div className="d-flex gap-2">
            <input
              className="form-control form-control-sm"
              type="time"
              value={cfg.quiet?.from || '23:00'}
              onChange={(e) => setCfg({ ...cfg, quiet: { ...cfg.quiet, from: e.target.value } })}
            />
            <input
              className="form-control form-control-sm"
              type="time"
              value={cfg.quiet?.to || '07:00'}
              onChange={(e) => setCfg({ ...cfg, quiet: { ...cfg.quiet, to: e.target.value } })}
            />
            <select
              className="form-select form-select-sm"
              value={cfg.quiet?.effect || 'off'}
              onChange={(e) => setCfg({ ...cfg, quiet: { ...cfg.quiet, effect: e.target.value } })}
            >
              {['off', 'warm', 'white', 'breathe'].map((ef) => (
                <option key={ef} value={ef}>
                  {EFFECT_LABELS[ef] || ef}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="col-md-5">
          <div className="form-check mb-1">
            <input
              className="form-check-input"
              type="checkbox"
              checked={!!cfg.active?.enabled}
              onChange={(e) => setCfg({ ...cfg, active: { ...cfg.active, enabled: e.target.checked } })}
            />
            <label className="form-check-label small">Only between (night light)</label>
          </div>
          <div className="d-flex gap-2">
            <input
              className="form-control form-control-sm"
              type="time"
              value={cfg.active?.from || '18:00'}
              onChange={(e) => setCfg({ ...cfg, active: { ...cfg.active, from: e.target.value } })}
            />
            <input
              className="form-control form-control-sm"
              type="time"
              value={cfg.active?.to || '23:00'}
              onChange={(e) => setCfg({ ...cfg, active: { ...cfg.active, to: e.target.value } })}
            />
          </div>
        </div>
      </div>
      {binding.last_error && <div className="small text-danger">{binding.last_error}</div>}
      {!online && <div className="small muted">Device offline — auto paused, Apply needs online.</div>}
    </div>
  );
}

function ScheduleBinding({
  deviceId,
  binding,
  online,
  platformTz,
  busy,
  setBusy,
  onChanged,
}: {
  deviceId: number;
  binding: DevicePluginBinding;
  online: boolean;
  platformTz: string;
  busy: string;
  setBusy: (v: string) => void;
  onChanged: () => Promise<void>;
}) {
  const [cfg, setCfg] = useState<ScheduleConfig>(() => asSchedule(binding.config as ScheduleConfig));
  useEffect(() => {
    setCfg(asSchedule(binding.config as ScheduleConfig));
  }, [binding.id, binding.updated_at]);
  const key = `p-${binding.id}`;

  const save = async (next: ScheduleConfig, enabled = binding.enabled) => {
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

  const setJob = (i: number, patch: Partial<ScheduleJob>) => {
    const jobs = [...(cfg.jobs || [])];
    jobs[i] = { ...jobs[i], ...patch };
    setCfg({ ...cfg, jobs });
  };

  return (
    <div className="border border-secondary rounded p-2 mb-2">
      <div className="d-flex flex-wrap align-items-center gap-2 mb-2">
        <strong>{binding.name || 'Schedule'}</strong>
        <span className="small muted">{binding.description}</span>
        <span className={`badge ${binding.running ? 'text-bg-success' : 'text-bg-secondary'}`}>
          {binding.running ? 'running 10s' : binding.enabled ? (online ? 'idle' : 'offline') : 'disabled'}
        </span>
        <div className="form-check form-switch mb-0 ms-auto">
          <input
            className="form-check-input"
            type="checkbox"
            checked={binding.enabled}
            disabled={!!busy}
            onChange={(e) => void save(cfg, e.target.checked)}
          />
          <label className="form-check-label small">enabled</label>
        </div>
      </div>
      <div className="row g-2 align-items-end mb-2">
        <div className="col-md-4">
          <label className="form-label small mb-1">Timezone (empty = {platformTz})</label>
          <TzField listId={`tz-sched-${binding.id}`} value={cfg.tz || ''} onChange={(tz) => setCfg({ ...cfg, tz })} platformTz={platformTz} />
        </div>
      </div>
      {(cfg.jobs || []).map((j, i) => (
        <div key={i} className="border border-secondary rounded p-2 mb-2">
          <div className="row g-2 align-items-end">
            <div className="col-md-3">
              <label className="form-label small mb-1">Name</label>
              <input className="form-control form-control-sm" value={j.name || ''} onChange={(e) => setJob(i, { name: e.target.value })} />
            </div>
            <div className="col-md-2">
              <label className="form-label small mb-1">At</label>
              <input className="form-control form-control-sm" type="time" value={j.at || '23:00'} onChange={(e) => setJob(i, { at: e.target.value })} />
            </div>
            <div className="col-md-3">
              <label className="form-label small mb-1">RPC</label>
              <input className="form-control form-control-sm" value={j.rpc} onChange={(e) => setJob(i, { rpc: e.target.value })} />
            </div>
            <div className="col-md-4 d-flex gap-2">
              <button
                type="button"
                className="btn btn-outline-danger btn-sm"
                disabled={!!busy}
                onClick={() => {
                  const jobs = (cfg.jobs || []).filter((_, k) => k !== i);
                  const next = { ...cfg, jobs };
                  setCfg(next);
                  void save(next);
                }}
              >
                Delete job
              </button>
            </div>
          </div>
          <div className="d-flex flex-wrap gap-2 mt-2 mb-1">
            {WEEKDAYS.map((w) => {
              const on = !j.days?.length || j.days.includes(w.d);
              return (
                <label key={w.d} className="form-check form-check-inline mb-0">
                  <input
                    className="form-check-input"
                    type="checkbox"
                    checked={on}
                    onChange={() => {
                      const cur = j.days?.length ? [...j.days] : WEEKDAYS.map((x) => x.d);
                      const nextDays = cur.includes(w.d) ? cur.filter((d) => d !== w.d) : [...cur, w.d].sort();
                      setJob(i, { days: nextDays.length === 7 ? [] : nextDays });
                    }}
                  />
                  <span className="form-check-label small">{w.label}</span>
                </label>
              );
            })}
            <span className="small muted">empty / all = every day</span>
          </div>
          <label className="form-label small mb-1">Params JSON</label>
          <textarea
            className="form-control form-control-sm mono"
            rows={2}
            key={`${binding.updated_at}-${i}`}
            defaultValue={JSON.stringify(j.params || {})}
            onBlur={(e) => {
              try {
                setJob(i, { params: JSON.parse(e.target.value || '{}') });
              } catch {
                alert('Invalid JSON params');
              }
            }}
          />
        </div>
      ))}
      <div className="d-flex gap-2">
        <button
          type="button"
          className="btn btn-outline-secondary btn-sm"
          disabled={!!busy}
          onClick={() => {
            const jobs = [...(cfg.jobs || []), { name: '', at: '07:00', days: [], rpc: 'reboot', params: {} }];
            const next = { ...cfg, jobs };
            setCfg(next);
            void save(next);
          }}
        >
          Add job
        </button>
        <button type="button" className="btn btn-primary btn-sm" disabled={!!busy} onClick={() => void save(cfg)}>
          Save
        </button>
        <button type="button" className="btn btn-outline-danger btn-sm" disabled={!!busy} onClick={() => void remove()}>
          Remove plugin
        </button>
      </div>
      {binding.last_error && <div className="small text-danger mt-2">{binding.last_error}</div>}
      {!online && <div className="small muted mt-1">Device offline — schedule paused.</div>}
    </div>
  );
}

function GenericBinding({
  binding,
  busy,
  onRemove,
  onSave,
}: {
  binding: DevicePluginBinding;
  busy: boolean;
  onRemove: () => void;
  onSave: (enabled: boolean, config: unknown) => Promise<void>;
}) {
  const [text, setText] = useState(() => JSON.stringify(binding.config || {}, null, 2));
  useEffect(() => {
    setText(JSON.stringify(binding.config || {}, null, 2));
  }, [binding.id, binding.updated_at]);

  return (
    <div className="border border-secondary rounded p-2 mb-2">
      <div className="d-flex flex-wrap align-items-center gap-2 mb-2">
        <strong>{binding.name || binding.plugin_id}</strong>
        <span className="small muted">{binding.description}</span>
        <div className="form-check form-switch mb-0 ms-auto">
          <input
            className="form-check-input"
            type="checkbox"
            checked={binding.enabled}
            disabled={busy}
            onChange={(e) => void onSave(e.target.checked, binding.config ?? {})}
          />
          <label className="form-check-label small">enabled</label>
        </div>
      </div>
      <textarea className="form-control form-control-sm mono mb-2" rows={8} value={text} onChange={(e) => setText(e.target.value)} />
      <div className="d-flex gap-2">
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={busy}
          onClick={() => {
            try {
              void onSave(binding.enabled, JSON.parse(text));
            } catch {
              alert('Invalid JSON');
            }
          }}
        >
          Save
        </button>
        <button type="button" className="btn btn-outline-danger btn-sm" disabled={busy} onClick={onRemove}>
          Remove plugin
        </button>
      </div>
    </div>
  );
}
