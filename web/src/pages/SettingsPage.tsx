import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { formatAge } from '../utils/format';
import type { APIToken, ExternalPlugin, ExternalPluginRegistration } from '../types';

export function SettingsPage() {
  const { settings, user } = useAuth();
  const s = settings || ({} as NonNullable<typeof settings>);
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [name, setName] = useState('mcp');
  const [created, setCreated] = useState<APIToken | null>(null);
  const [busy, setBusy] = useState(false);
  const [plugins, setPlugins] = useState<ExternalPlugin[]>([]);
  const [newPlugin, setNewPlugin] = useState<ExternalPluginRegistration | null>(null);
  const [pluginForm, setPluginForm] = useState({ id: '', name: '', description: '' });
  const isAdmin = user?.role === 'admin';

  const load = () => {
    api<APIToken[]>('/me/tokens')
      .then(setTokens)
      .catch((e) => alert(e instanceof Error ? e.message : String(e)));
  };

  const loadPlugins = () => {
    api<ExternalPlugin[]>('/plugins/external')
      .then(setPlugins)
      .catch((e) => console.error('Failed to load plugins:', e));
  };

  useEffect(() => {
    load();
    if (isAdmin) loadPlugins();
  }, [isAdmin]);

  return (
    <div>
      <h1 className="h3 mb-3">Settings</h1>
      <div className="panel mono small mb-4">
        <div>Platform: {s.platform_name}</div>
        <div>Retention: {s.telemetry_retention_days} days</div>
        <div>RPC timeout: {s.rpc_default_timeout_ms} ms</div>
        <div>
          Broker: {s.broker_host}:{s.mqtt_port || 1883}
          {s.mqtt_ws_url ? <> · WS {s.mqtt_ws_url}</> : null} · <Link to="/mqtt">MQTT monitor</Link>
        </div>
        <div>Rules bus: {String(s.rules_bus_enabled)}</div>
        <div>Node-RED: {s.nodered_url}</div>
        <div className="mt-2">Public URL: {s.public_base_url || '—'}</div>
        <div>Timezone: {s.timezone || 'UTC'}</div>
        <div>Registration: {String(s.register_enabled)}</div>
        <div>Email verify: {String(s.require_email_verify)}</div>
        <div>Google OAuth: {String(s.google_enabled)}</div>
      </div>

      <h2 className="h5 mb-2">API tokens</h2>
      <p className="muted small mb-3">
        Long-lived tokens for MCP and scripts. Use as <span className="mono">Authorization: Bearer …</span>. Shown once at create.
      </p>

      <form
        className="d-flex flex-wrap gap-2 mb-3"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            const tok = await api<APIToken>('/me/tokens', {
              method: 'POST',
              body: JSON.stringify({ name: name.trim() || 'mcp' }),
            });
            setCreated(tok);
            setName('mcp');
            await load();
          } catch (err) {
            alert(err instanceof Error ? err.message : String(err));
          } finally {
            setBusy(false);
          }
        }}
      >
        <input
          className="form-control form-control-sm"
          style={{ maxWidth: 220 }}
          placeholder="Name (e.g. mcp)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={64}
        />
        <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
          Create token
        </button>
      </form>

      {created?.token && (
        <>
          <div className="modal-backdrop fade show" />
          <div className="modal fade show d-block" tabIndex={-1} role="dialog">
            <div className="modal-dialog modal-dialog-centered">
              <div className="modal-content">
                <div className="modal-header">
                  <h5 className="modal-title">API token created</h5>
                  <button type="button" className="btn-close btn-close-white" onClick={() => setCreated(null)} />
                </div>
                <div className="modal-body">
                  <p className="text-danger small">Copy now — it will not be shown again.</p>
                  <p className="muted small mb-1">
                    Name: <span className="mono">{created.name}</span>
                  </p>
                  <pre className="bg-dark border border-secondary rounded p-2 small mono mb-2 user-select-all">
                    {created.token}
                  </pre>
                  <button
                    type="button"
                    className="btn btn-outline-secondary btn-sm"
                    onClick={() => void navigator.clipboard.writeText(created.token!)}
                  >
                    Copy
                  </button>
                </div>
                <div className="modal-footer">
                  <button type="button" className="btn btn-primary" onClick={() => setCreated(null)}>
                    Done
                  </button>
                </div>
              </div>
            </div>
          </div>
        </>
      )}

      <div className="panel table-responsive">
        <table className="table table-sm align-middle mb-0">
          <thead>
            <tr>
              <th>Name</th>
              <th>Prefix</th>
              <th>Created</th>
              <th>Last used</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {tokens.length === 0 && (
              <tr>
                <td colSpan={5} className="muted small">
                  No tokens yet.
                </td>
              </tr>
            )}
            {tokens.map((t) => (
              <tr key={t.id}>
                <td>{t.name}</td>
                <td className="mono">{t.token_prefix}…</td>
                <td className="small">{formatAge(t.created_at)}</td>
                <td className="small">{t.last_used_at ? formatAge(t.last_used_at) : '—'}</td>
                <td className="text-end">
                  <button
                    type="button"
                    className="btn btn-outline-danger btn-sm"
                    onClick={async () => {
                      if (!confirm(`Revoke token “${t.name}”?`)) return;
                      try {
                        await api(`/me/tokens/${t.id}`, { method: 'DELETE' });
                        await load();
                      } catch (err) {
                        alert(err instanceof Error ? err.message : String(err));
                      }
                    }}
                  >
                    Revoke
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {isAdmin && (
        <>
          <h2 className="h5 mt-4 mb-2">External Plugins</h2>
          <p className="muted small mb-3">
            Plugins that connect via MQTT or webhook. Credentials are shown only at creation.
          </p>

          <form
            className="d-flex flex-wrap gap-2 mb-3"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              try {
                const result = await api<ExternalPluginRegistration>('/plugins/external', {
                  method: 'POST',
                  body: JSON.stringify(pluginForm),
                });
                setNewPlugin(result);
                setPluginForm({ id: '', name: '', description: '' });
                await loadPlugins();
              } catch (err) {
                alert(err instanceof Error ? err.message : String(err));
              } finally {
                setBusy(false);
              }
            }}
          >
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 160 }}
              placeholder="Plugin ID"
              value={pluginForm.id}
              onChange={(e) => setPluginForm({ ...pluginForm, id: e.target.value })}
              required
            />
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 200 }}
              placeholder="Name"
              value={pluginForm.name}
              onChange={(e) => setPluginForm({ ...pluginForm, name: e.target.value })}
              required
            />
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 260 }}
              placeholder="Description (optional)"
              value={pluginForm.description}
              onChange={(e) => setPluginForm({ ...pluginForm, description: e.target.value })}
            />
            <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
              Create plugin
            </button>
          </form>

          {newPlugin && (
            <div className="modal-backdrop fade show" />
          )}
          {newPlugin && (
            <div className="modal fade show d-block" tabIndex={-1} role="dialog" style={{ zIndex: 1050 }}>
              <div className="modal-dialog modal-dialog-centered modal-lg">
                <div className="modal-content">
                  <div className="modal-header">
                    <h5 className="modal-title">Plugin created — save credentials!</h5>
                    <button type="button" className="btn-close btn-close-white" onClick={() => setNewPlugin(null)} />
                  </div>
                  <div className="modal-body">
                    <p className="text-danger small mb-2">
                      Copy these credentials now — the password will NOT be shown again.
                    </p>
                    <div className="mono small">
                      <div className="mb-2">
                        <span className="muted">Plugin ID:</span> {newPlugin.id}
                      </div>
                      <div className="mb-2">
                        <span className="muted">API Token:</span>
                        <pre className="bg-dark border border-secondary rounded p-2 small mono mb-0 mt-1 user-select-all text-break">
                          {newPlugin.token}
                        </pre>
                      </div>
                      <div className="mb-2">
                        <span className="muted">MQTT User:</span> <span className="user-select-all">{newPlugin.mqtt_user}</span>
                      </div>
                      <div className="mb-2">
                        <span className="muted">MQTT Password:</span>
                        <pre className="bg-dark border border-secondary rounded p-2 small mono mb-0 mt-1 user-select-all text-break">
                          {newPlugin.mqtt_pass}
                        </pre>
                      </div>
                      <div className="mb-2">
                        <span className="muted">MQTT Broker:</span> {newPlugin.mqtt_host}:{newPlugin.mqtt_port}
                      </div>
                      <div>
                        <span className="muted">Rules Bus:</span> {newPlugin.rules_bus}
                      </div>
                    </div>
                  </div>
                  <div className="modal-footer">
                    <button
                      type="button"
                      className="btn btn-outline-secondary btn-sm"
                      onClick={() => {
                        const text = `Plugin: ${newPlugin.id}\nToken: ${newPlugin.token}\nMQTT User: ${newPlugin.mqtt_user}\nMQTT Pass: ${newPlugin.mqtt_pass}\nBroker: ${newPlugin.mqtt_host}:${newPlugin.mqtt_port}`;
                        void navigator.clipboard.writeText(text);
                      }}
                    >
                      Copy all
                    </button>
                    <button type="button" className="btn btn-primary" onClick={() => setNewPlugin(null)}>
                      Done
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}

          <div className="panel table-responsive">
            <table className="table table-sm align-middle mb-0">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Mode</th>
                  <th>Status</th>
                  <th>MQTT User</th>
                  <th>Created</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {plugins.length === 0 && (
                  <tr>
                    <td colSpan={7} className="muted small">
                      No external plugins.
                    </td>
                  </tr>
                )}
                {plugins.map((p) => (
                  <tr key={p.id}>
                    <td className="mono">{p.id}</td>
                    <td>{p.name}</td>
                    <td>{p.mode}</td>
                    <td>
                      <span className={`badge ${p.status === 'online' ? 'text-bg-success' : 'text-bg-secondary'}`}>
                        {p.status}
                      </span>
                    </td>
                    <td className="mono small">{p.mqtt_user}</td>
                    <td className="small">{formatAge(p.created_at)}</td>
                    <td className="text-end">
                      <button
                        type="button"
                        className="btn btn-outline-warning btn-sm me-1"
                        onClick={async () => {
                          if (!confirm(`Regenerate MQTT password for ${p.name}?`)) return;
                          try {
                            const result = await api<{ mqtt_pass: string }>(`/plugins/external/${p.id}/regenerate-mqtt`, { method: 'POST' });
                            alert(`New MQTT password:\n${result.mqtt_pass}\n\nSave it now!`);
                          } catch (err) {
                            alert(err instanceof Error ? err.message : String(err));
                          }
                        }}
                      >
                        Regen MQTT
                      </button>
                      <button
                        type="button"
                        className="btn btn-outline-danger btn-sm"
                        onClick={async () => {
                          if (!confirm(`Delete plugin ${p.name}?`)) return;
                          try {
                            await api(`/plugins/external/${p.id}`, { method: 'DELETE' });
                            await loadPlugins();
                          } catch (err) {
                            alert(err instanceof Error ? err.message : String(err));
                          }
                        }}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
