import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { formatAge } from '../utils/format';
import type { APIToken } from '../types';

export function SettingsPage() {
  const { settings } = useAuth();
  const s = settings || ({} as NonNullable<typeof settings>);
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [name, setName] = useState('mcp');
  const [created, setCreated] = useState<APIToken | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    api<APIToken[]>('/me/tokens')
      .then(setTokens)
      .catch((e) => alert(e instanceof Error ? e.message : String(e)));
  };

  useEffect(() => {
    load();
  }, []);

  return (
    <div>
      <h1 className="h3 mb-3">Settings</h1>
      <div className="panel mono small mb-4">
        <div>Platform: {s.platform_name}</div>
        <div>Retention: {s.telemetry_retention_days} days</div>
        <div>RPC timeout: {s.rpc_default_timeout_ms} ms</div>
        <div>
          Broker: {s.broker_host}:1883 · <Link to="/mqtt">MQTT monitor</Link>
        </div>
        <div>Rules bus: {String(s.rules_bus_enabled)}</div>
        <div>Node-RED: {s.nodered_url}</div>
        <div>Public URL: {s.public_base_url || '—'}</div>
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
    </div>
  );
}
