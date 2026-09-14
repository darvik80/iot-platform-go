import { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { errorMsg, useToast } from '../components/Toast';
import { useT } from '../i18n/I18nContext';
import type { PluginPolicy } from '../types';

type Tab = 'integrations' | 'account';

export function AccountSettingsPage() {
  const { activeMembership } = useAuth();
  const toast = useToast();
  const t = useT();
  const [tab, setTab] = useState<Tab>('integrations');
  const [busy, setBusy] = useState(false);

  // Integrations
  const [policies, setPolicies] = useState<PluginPolicy[]>([]);
  const [settingsPlugin, setSettingsPlugin] = useState<PluginPolicy | null>(null);
  const [settingsJSON, setSettingsJSON] = useState('');

  if (activeMembership?.role !== 'admin') return <Navigate to="/" replace />;

  const loadPolicies = () => api<PluginPolicy[]>('/plugins/policies').then(setPolicies).catch(console.error);

  useEffect(() => {
    loadPolicies();
  }, []);

  const handleTogglePolicy = async (pluginId: string, enabled: boolean) => {
    try {
      await api(`/plugins/policies/${pluginId}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      });
      await loadPolicies();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  };

  const handleOpenSettings = (p: PluginPolicy) => {
    setSettingsPlugin(p);
    try {
      const parsed = JSON.parse(p.settings_json || '{}');
      setSettingsJSON(JSON.stringify(parsed, null, 2));
    } catch {
      setSettingsJSON('{}');
    }
  };

  const handleSaveSettings = async () => {
    if (!settingsPlugin) return;
    setBusy(true);
    try {
      let parsed: Record<string, unknown>;
      try {
        parsed = JSON.parse(settingsJSON);
      } catch {
        toast.warning('Invalid JSON');
        setBusy(false);
        return;
      }
      await api(`/plugins/policies/${settingsPlugin.id}`, {
        method: 'PATCH',
        body: JSON.stringify({ settings: parsed }),
      });
      setSettingsPlugin(null);
      await loadPolicies();
    } catch (err) {
      toast.error(errorMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <h1 className="h3 mb-3">{t('nav.account')}</h1>

      <ul className="nav nav-tabs mb-3">
        {(['integrations', 'account'] as Tab[]).map((tabKey) => (
          <li key={tabKey} className="nav-item">
            <button
              type="button"
              className={`nav-link${tab === tabKey ? ' active' : ''}`}
              onClick={() => setTab(tabKey)}
            >
              {tabKey === 'integrations' ? t('account.integrations') : t('account.accountInfo')}
            </button>
          </li>
        ))}
      </ul>

      {/* ── Integrations tab ────────────────────────────────────────────── */}
      {tab === 'integrations' && (
        <div>
          <div className="d-flex align-items-center gap-2 mb-3">
            <p className="muted small mb-0">
              Manage catalog plugins for this account. Disabled plugins stop running for all devices in this account.
            </p>
            <span
              className="help-icon"
              title="Catalog plugins are built-in plugins that can be enabled/disabled per account. Each plugin may have its own settings."
            >
              i
            </span>
          </div>
          <div className="d-flex flex-column gap-2">
            {policies.map((p) => (
              <div key={p.id} className="panel d-flex justify-content-between align-items-center">
                <div className="flex-grow-1">
                  <div className="d-flex align-items-center gap-2">
                    <strong>{p.name}</strong>
                    <span className={`badge ${p.enabled ? 'badge-online' : 'badge-offline'}`}>
                      {p.enabled ? 'enabled' : 'disabled'}
                    </span>
                  </div>
                  <div className="muted small mt-1">{p.description}</div>
                </div>
                <div className="d-flex gap-2 ms-3">
                  <button
                    type="button"
                    className="btn btn-outline-secondary btn-sm"
                    title="Edit plugin-specific configuration settings"
                    onClick={() => handleOpenSettings(p)}
                  >
                    Settings
                  </button>
                  <div className="form-check form-switch d-flex align-items-center">
                    <input
                      className="form-check-input"
                      type="checkbox"
                      checked={p.enabled}
                      onChange={(e) => handleTogglePolicy(p.id, e.target.checked)}
                      title={p.enabled ? 'Disable this plugin for your account' : 'Enable this plugin for your account'}
                    />
                  </div>
                </div>
              </div>
            ))}
            {policies.length === 0 && <div className="muted small">No catalog plugins registered.</div>}
          </div>
        </div>
      )}

      {/* ── Account tab ─────────────────────────────────────────────────── */}
      {tab === 'account' && (
        <div className="panel mono small">
          <div>Account: {activeMembership?.account_name}</div>
          <div>Slug: {activeMembership?.account_slug}</div>
          <div>Status: {activeMembership?.account_status}</div>
          <div>Your role: {activeMembership?.role}</div>
        </div>
      )}

      {/* ── Settings modal ──────────────────────────────────────────────── */}
      {settingsPlugin && (
        <>
          <div className="modal-backdrop fade show" />
          <div className="modal fade show d-block" tabIndex={-1} role="dialog" style={{ zIndex: 1050 }}>
            <div className="modal-dialog modal-dialog-centered modal-lg">
              <div className="modal-content">
                <div className="modal-header">
                  <h5 className="modal-title">Settings: {settingsPlugin.name}</h5>
                  <button type="button" className="btn-close btn-close-white" onClick={() => setSettingsPlugin(null)} />
                </div>
                <div className="modal-body">
                  <p className="muted small mb-2">Edit the JSON configuration for this plugin.</p>
                  <textarea
                    className="form-control mono small"
                    rows={12}
                    value={settingsJSON}
                    onChange={(e) => setSettingsJSON(e.target.value)}
                  />
                </div>
                <div className="modal-footer">
                  <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => setSettingsPlugin(null)}>
                    {t('common.cancel')}
                  </button>
                  <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={handleSaveSettings}>
                    {t('common.save')}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
