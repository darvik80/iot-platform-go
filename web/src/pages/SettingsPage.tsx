import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { errorMsg, useToast } from '../components/Toast';
import { formatAge } from '../utils/format';
import { useT } from '../i18n/I18nContext';
import type { APIToken, ExternalPlugin, ExternalPluginRegistration } from '../types';

interface RegenResult {
  pluginId: string;
  pluginName: string;
  mqtt_pass: string;
  mqtt_user: string;
  mqtt_host: string;
  mqtt_port: number;
  rules_bus: string;
}

export function SettingsPage() {
  const { settings, activeMembership } = useAuth();
  const s = settings || ({} as NonNullable<typeof settings>);
  const toast = useToast();
  const t = useT();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [name, setName] = useState('mcp');
  const [created, setCreated] = useState<APIToken | null>(null);
  const [busy, setBusy] = useState(false);
  const [plugins, setPlugins] = useState<ExternalPlugin[]>([]);
  const [newPlugin, setNewPlugin] = useState<ExternalPluginRegistration | null>(null);
  const [regenResult, setRegenResult] = useState<RegenResult | null>(null);
  const [pluginForm, setPluginForm] = useState({ id: '', name: '', description: '' });
  const isAdmin = activeMembership?.role === 'admin';
  const accountSlug = activeMembership?.account_slug || '{account_slug}';

  const load = () => {
    api<APIToken[]>('/me/tokens')
      .then(setTokens)
      .catch((e) => toast.error(errorMsg(e)));
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
      <h1 className="h3 mb-3">{t('settings.title')}</h1>
      <div className="panel mono small mb-4">
        <div>Platform: {s.platform_name}</div>
        <div>Retention: {s.telemetry_retention_days} days</div>
        <div>RPC timeout: {s.rpc_default_timeout_ms} ms</div>
        <div>
          Broker: {s.broker_host}:{s.mqtt_port || 1883}
          {s.mqtt_ws_url ? <> · WS {s.mqtt_ws_url}</> : null} · <Link to="/mqtt">MQTT monitor</Link>
        </div>
        <div>Rules bus: {String(s.rules_bus_enabled)}</div>
        <div className="mt-2">Public URL: {s.public_base_url || '—'}</div>
        <div>Timezone: {s.timezone || 'UTC'}</div>
        <div>Registration: {String(s.register_enabled)}</div>
        <div>Email verify: {String(s.require_email_verify)}</div>
        <div>Google OAuth: {String(s.google_enabled)}</div>
      </div>

      <h2 className="h5 mb-2">{t('settings.apiTokens')}</h2>
      <p className="muted small mb-3">
        {t('settings.apiTokensDescription')}
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
            toast.error(errorMsg(err));
          } finally {
            setBusy(false);
          }
        }}
      >
        <input
          className="form-control form-control-sm"
          style={{ maxWidth: 220 }}
          placeholder={t('settings.tokenName')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={64}
        />
        <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
          {t('settings.createToken')}
        </button>
      </form>

      {created?.token && (
        <>
          <div className="modal-backdrop fade show" />
          <div className="modal fade show d-block" tabIndex={-1} role="dialog">
            <div className="modal-dialog modal-dialog-centered">
              <div className="modal-content">
                <div className="modal-header">
                  <h5 className="modal-title">{t('settings.tokenCreated')}</h5>
                  <button type="button" className="btn-close btn-close-white" onClick={() => setCreated(null)} />
                </div>
                <div className="modal-body">
                  <p className="text-danger small">{t('settings.copyNow')}</p>
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
                    {t('common.copy')}
                  </button>
                </div>
                <div className="modal-footer">
                  <button type="button" className="btn btn-primary" onClick={() => setCreated(null)}>
                    {t('common.done')}
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
              <th>{t('common.name')}</th>
              <th>{t('settings.prefix')}</th>
              <th>{t('settings.created')}</th>
              <th>{t('settings.lastUsed')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {tokens.length === 0 && (
              <tr>
                <td colSpan={5} className="muted small">
                  {t('settings.noTokens')}
                </td>
              </tr>
            )}
            {tokens.map((tok) => (
              <tr key={tok.id}>
                <td>{tok.name}</td>
                <td className="mono">{tok.token_prefix}…</td>
                <td className="small">{formatAge(tok.created_at)}</td>
                <td className="small">{tok.last_used_at ? formatAge(tok.last_used_at) : '—'}</td>
                <td className="text-end">
                  <button
                    type="button"
                    className="btn btn-outline-danger btn-sm"
                    onClick={async () => {
                      if (!confirm(t('settings.revokeConfirm', { name: tok.name }))) return;
                      try {
                        await api(`/me/tokens/${tok.id}`, { method: 'DELETE' });
                        await load();
                      } catch (err) {
                        toast.error(errorMsg(err));
                      }
                    }}
                  >
                    {t('settings.revoke')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {isAdmin && (
        <>
          <h2 className="h5 mt-4 mb-2">{t('settings.integrations')}</h2>
          <div className="panel mb-3">
            <div className="d-flex align-items-start gap-2 mb-3">
              <p className="muted small mb-0">
                {t('settings.integrationsDescription')}
              </p>
              <span className="help-icon" title="Connect external services like Node-RED via MQTT rules bus. Each external plugin gets its own credentials and can subscribe to account-scoped events.">i</span>
            </div>

            {/* Rules bus info */}
            {s.rules_bus_enabled ? (
              <>
                <p className="small muted mb-2">
                  Events topic template:{' '}
                  <span className="mono">
                    {s.rules_bus_prefix || 'platform/v1'}/events/{activeMembership?.account_slug || '{account_slug}'}/&#123;product&#125;/&#123;device&#125;/&#123;kind&#125;
                  </span>
                </p>
                <button
                  type="button"
                  className="btn btn-outline-secondary btn-sm mb-3"
                  onClick={() => {
                    const topic = `${s.rules_bus_prefix || 'platform/v1'}/events/${activeMembership?.account_slug || '{account_slug}'}/{product}/{device}/{kind}`;
                    void navigator.clipboard.writeText(topic);
                  }}
                >
                  {t('settings.copyMqttTemplate')}
                </button>
              </>
            ) : (
              <p className="small muted mb-3">{t('settings.rulesBusDisabled')}</p>
            )}
            <div className="small">
              See <Link to="/docs#automation" className="muted">automation documentation</Link> for Node-RED integration examples.
            </div>
          </div>

          <h3 className="h6 mt-3 mb-2">{t('settings.externalPlugins')}</h3>
          <p className="muted small mb-3">
            {t('settings.externalPluginsDescription')}
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
                toast.error(errorMsg(err));
              } finally {
                setBusy(false);
              }
            }}
          >
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 160 }}
              placeholder={t('settings.pluginId')}
              value={pluginForm.id}
              onChange={(e) => setPluginForm({ ...pluginForm, id: e.target.value })}
              required
              title="Unique identifier for the plugin (e.g., my-plugin)"
            />
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 200 }}
              placeholder="Name"
              value={pluginForm.name}
              onChange={(e) => setPluginForm({ ...pluginForm, name: e.target.value })}
              required
              title="Human-readable name for the plugin"
            />
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 260 }}
              placeholder={t('settings.pluginDescription')}
              value={pluginForm.description}
              onChange={(e) => setPluginForm({ ...pluginForm, description: e.target.value })}
              title="Optional description of what the plugin does"
            />
            <button type="submit" className="btn btn-primary btn-sm" disabled={busy} title="Create a new external plugin">
              {t('settings.createPlugin')}
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
                    <h5 className="modal-title">{t('settings.pluginCreated')}</h5>
                    <button type="button" className="btn-close btn-close-white" onClick={() => setNewPlugin(null)} />
                  </div>
                  <div className="modal-body">
                    <p className="text-danger small mb-2">
                      {t('settings.copyCredentialsNow')}
                    </p>
                    <div className="mono small">
                      <div className="mb-2">
                        <span className="muted">Plugin ID:</span> {newPlugin.id}
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
                      <div className="mb-3">
                        <span className="muted">Rules Bus:</span> {newPlugin.rules_bus}
                      </div>
                      <hr className="border-secondary my-2" />
                      <div className="mb-2">
                        <span className="muted">Subscribe (events):</span>
                        <div className="mono small mt-1 user-select-all">{newPlugin.rules_bus}/events/{accountSlug}/&#123;product&#125;/&#123;device&#125;/&#123;kind&#125;</div>
                      </div>
                      <div>
                        <span className="muted">Publish (commands):</span>
                        <div className="mono small mt-1 user-select-all">{newPlugin.rules_bus}/commands/{accountSlug}/&#123;product&#125;/&#123;device&#125;/&#123;kind&#125;</div>
                      </div>
                    </div>
                  </div>
                  <div className="modal-footer">
                    <button
                      type="button"
                      className="btn btn-outline-secondary btn-sm"
                      onClick={() => {
                        const text = `Plugin: ${newPlugin.id}\nMQTT User: ${newPlugin.mqtt_user}\nMQTT Pass: ${newPlugin.mqtt_pass}\nBroker: ${newPlugin.mqtt_host}:${newPlugin.mqtt_port}\nSubscribe: ${newPlugin.rules_bus}/events/${accountSlug}/{product}/{device}/{kind}\nPublish: ${newPlugin.rules_bus}/commands/${accountSlug}/{product}/{device}/{kind}`;
                        void navigator.clipboard.writeText(text);
                      }}
                    >
                      {t('settings.copyAll')}
                    </button>
                    <button type="button" className="btn btn-primary" onClick={() => setNewPlugin(null)}>
                      {t('common.done')}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}

          {regenResult && (
            <>
              <div className="modal-backdrop fade show" />
              <div className="modal fade show d-block" tabIndex={-1} role="dialog" style={{ zIndex: 1050 }}>
                <div className="modal-dialog modal-dialog-centered">
                  <div className="modal-content">
                    <div className="modal-header">
                      <h5 className="modal-title">{t('settings.passwordRegenerated')}</h5>
                      <button type="button" className="btn-close btn-close-white" onClick={() => setRegenResult(null)} />
                    </div>
                    <div className="modal-body">
                      <p className="text-danger small mb-2">{t('settings.saveNewPassword')}</p>
                      <div className="mono small">
                        <div className="mb-2">
                          <span className="muted">Plugin:</span> {regenResult.pluginName} <span className="muted">({regenResult.pluginId})</span>
                        </div>
                        <div className="mb-2">
                          <span className="muted">MQTT User:</span> <span className="user-select-all">{regenResult.mqtt_user}</span>
                        </div>
                        <div className="mb-2">
                          <span className="muted">MQTT Password:</span>
                          <pre className="bg-dark border border-secondary rounded p-2 small mono mb-0 mt-1 user-select-all text-break">
                            {regenResult.mqtt_pass}
                          </pre>
                        </div>
                        <div className="mb-2">
                          <span className="muted">Broker:</span> {regenResult.mqtt_host}:{regenResult.mqtt_port}
                        </div>
                        <hr className="border-secondary my-2" />
                        <div className="mb-2">
                          <span className="muted">Subscribe (events):</span>
                          <div className="mono small mt-1 user-select-all">platform/v1/events/{accountSlug}/&#123;product&#125;/&#123;device&#125;/&#123;kind&#125;</div>
                        </div>
                        <div>
                          <span className="muted">Publish (commands):</span>
                          <div className="mono small mt-1 user-select-all">platform/v1/commands/{accountSlug}/&#123;product&#125;/&#123;device&#125;/&#123;kind&#125;</div>
                        </div>
                      </div>
                    </div>
                    <div className="modal-footer">
                      <button
                        type="button"
                        className="btn btn-outline-secondary btn-sm"
                        onClick={() => {
                          const text = `Plugin: ${regenResult.pluginId}\nMQTT User: ${regenResult.mqtt_user}\nMQTT Pass: ${regenResult.mqtt_pass}\nBroker: ${regenResult.mqtt_host}:${regenResult.mqtt_port}`;
                          void navigator.clipboard.writeText(text);
                        }}
                      >
                        {t('settings.copyCredentials')}
                      </button>
                      <button type="button" className="btn btn-primary" onClick={() => setRegenResult(null)}>
                        {t('common.done')}
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
                  <th>{t('common.id')}</th>
                  <th>{t('common.name')}</th>
                  <th>{t('common.status')}</th>
                  <th>{t('settings.mqttUser')}</th>
                  <th>{t('settings.created')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {plugins.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted small">
                      {t('settings.noExternalPlugins')}
                    </td>
                  </tr>
                )}
                {plugins.map((p) => (
                  <tr key={p.id}>
                    <td className="mono">{p.id}</td>
                    <td>{p.name}</td>
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
                          if (!confirm(t('settings.regenConfirm', { name: p.name }))) return;
                          try {
                            const result = await api<{ mqtt_pass: string; mqtt_user: string }>(`/plugins/external/${p.id}/regenerate-mqtt`, { method: 'POST' });
                            setRegenResult({
                              pluginId: p.id,
                              pluginName: p.name,
                              mqtt_pass: result.mqtt_pass,
                              mqtt_user: result.mqtt_user,
                              mqtt_host: s.broker_host || '',
                              mqtt_port: s.mqtt_port || 1883,
                              rules_bus: s.rules_bus_enabled ? (s.rules_bus_prefix || 'platform/v1') : '',
                            });
                          } catch (err) {
                            toast.error(errorMsg(err));
                          }
                        }}
                      >
                        {t('settings.regenMqtt')}
                      </button>
                      <button
                        type="button"
                        className="btn btn-outline-danger btn-sm"
                        onClick={async () => {
                          if (!confirm(t('settings.deletePluginConfirm', { name: p.name }))) return;
                          try {
                            await api(`/plugins/external/${p.id}`, { method: 'DELETE' });
                            await loadPlugins();
                          } catch (err) {
                            toast.error(errorMsg(err));
                          }
                        }}
                      >
                        {t('common.delete')}
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
