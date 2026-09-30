import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';

interface SetupState {
  current_step: number;
  database_driver: string;
  database_dsn: string;
  admin_login: string;
  admin_email: string;
  admin_password_set: boolean;
  account_slug: string;
  account_name: string;
  smtp_host: string;
  smtp_port: number;
  smtp_user: string;
  smtp_pass_set: boolean;
  smtp_from: string;
  jwt_secret_set: boolean;
  mqtt_tls_cert: string;
  mqtt_tls_key: string;
  mqtt_tls_ca?: string;
  public_base_url: string;
  broker_host_public: string;
  completed: boolean;
  error?: string;
}

const STEPS = [
  { num: 1, title: 'Database', icon: '⛁' },
  { num: 2, title: 'Admin', icon: '👤' },
  { num: 3, title: 'Account', icon: '🏢' },
  { num: 4, title: 'SMTP', icon: '✉' },
  { num: 5, title: 'JWT Secret', icon: '🔑' },
  { num: 6, title: 'MQTT TLS', icon: '🔒' },
  { num: 7, title: 'Public URL', icon: '🌐' },
  { num: 8, title: 'Review & Apply', icon: '✓' },
];

export function SetupPage() {
  const navigate = useNavigate();
  const { markSetupComplete } = useAuth();
  const [state, setState] = useState<SetupState | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [applying, setApplying] = useState(false);
  const [applied, setApplied] = useState(false);
  const [generatingTLS, setGeneratingTLS] = useState(false);

  const fetchState = useCallback(async () => {
    try {
      const s = await api<SetupState>('/setup/state');
      setState(s);
      if (s.completed) {
        markSetupComplete();
        setApplied(true);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [markSetupComplete]);

  useEffect(() => { fetchState(); }, [fetchState]);

  const submitStep = async (step: number, data: Record<string, unknown>) => {
    setSaving(true);
    setError('');
    try {
      await api('/setup/step', {
        method: 'POST',
        body: JSON.stringify({ step, data }),
      });
      await fetchState();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const applySetup = async () => {
    setApplying(true);
    setError('');
    try {
      await api('/setup/apply', { method: 'POST' });
      markSetupComplete();
      setApplied(true);
      await fetchState();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setApplying(false);
    }
  };

  const generateSecret = () => {
    const arr = new Uint8Array(32);
    crypto.getRandomValues(arr);
    return Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
  };

  if (loading) {
    return (
      <div className="login-wrap">
        <div className="login-box text-center">
          <p className="muted">Loading setup wizard…</p>
        </div>
      </div>
    );
  }

  if (applied) {
    return (
      <div className="login-wrap">
        <div className="login-box">
          <h1 className="h3 brand">
            <img className="brand-mark" src="/favicon.svg" alt="" />
            Setup <span>Complete</span>
          </h1>
          <div className="mt-3">
            <p className="text-success">Setup wizard completed successfully.</p>
            <p className="muted">Please restart the application to apply the configuration, then sign in.</p>
            <button className="btn btn-primary mt-2" onClick={() => navigate('/login')}>
              Go to Login
            </button>
          </div>
        </div>
      </div>
    );
  }

  const step = state?.current_step ?? 1;

  return (
    <div className="login-wrap">
      <div className="login-box" style={{ width: 'min(560px, 100%)' }}>
        <h1 className="h3 brand mb-1">
          <img className="brand-mark" src="/favicon.svg" alt="" />
          Platform <span>Setup</span>
        </h1>
        <p className="muted small mb-3">Initial configuration wizard</p>

        {/* Progress bar */}
        <div className="d-flex gap-1 mb-3">
          {STEPS.map((s) => (
            <div
              key={s.num}
              className="flex-fill text-center"
              style={{
                height: 4,
                borderRadius: 2,
                background: s.num < step ? 'var(--ok)' : s.num === step ? 'var(--accent)' : 'var(--border)',
                transition: 'background 0.3s',
              }}
              title={`Step ${s.num}: ${s.title}`}
            />
          ))}
        </div>
        <div className="d-flex justify-content-between mb-3" style={{ fontSize: '0.7rem' }}>
          {STEPS.map((s) => (
            <span
              key={s.num}
              className={s.num === step ? '' : 'muted'}
              style={{ color: s.num === step ? 'var(--accent)' : undefined }}
            >
              {s.num}
            </span>
          ))}
        </div>

        <h2 className="h6 mb-3">
          Step {step}: {STEPS[step - 1]?.title}
        </h2>

        {error && <div className="alert alert-danger py-2 px-3 small">{error}</div>}

        {/* Step 1: Database */}
        {step === 1 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(1, {
                database_driver: fd.get('database_driver'),
                database_dsn: fd.get('database_dsn'),
              });
            }}
            saving={saving}
          >
            <div>
              <label className="form-label">Database Driver</label>
              <select name="database_driver" className="form-select" defaultValue={state.database_driver}>
                <option value="sqlite">SQLite</option>
                <option value="postgres">PostgreSQL</option>
              </select>
              <div className="form-text">SQLite for single-file deployment, PostgreSQL for production</div>
            </div>
            <div>
              <label className="form-label">Connection String</label>
              <input
                name="database_dsn"
                className="form-control mono"
                defaultValue={state.database_dsn}
                placeholder="/opt/iot-platform/lib/platform.db or postgres://..."
              />
              <div className="form-text">
                SQLite: file path. PostgreSQL: <code className="code">postgres://user:pass@host:5432/dbname?sslmode=disable</code>
              </div>
            </div>
          </StepForm>
        )}

        {/* Step 2: Admin */}
        {step === 2 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(2, {
                admin_login: fd.get('admin_login'),
                admin_email: fd.get('admin_email'),
                admin_password: fd.get('admin_password'),
              });
            }}
            saving={saving}
          >
            <div>
              <label className="form-label">Admin Login</label>
              <input name="admin_login" className="form-control" defaultValue={state.admin_login} required pattern="[a-zA-Z][a-zA-Z0-9._-]{2,31}" />
            </div>
            <div>
              <label className="form-label">Admin Email</label>
              <input name="admin_email" type="email" className="form-control" defaultValue={state.admin_email} required />
            </div>
            <div>
              <label className="form-label">Admin Password</label>
              <input name="admin_password" type="password" className="form-control" minLength={8} required placeholder="Min 8 characters" />
            </div>
          </StepForm>
        )}

        {/* Step 3: Account */}
        {step === 3 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(3, {
                account_slug: fd.get('account_slug'),
                account_name: fd.get('account_name'),
              });
            }}
            saving={saving}
          >
            <div>
              <label className="form-label">Account Slug</label>
              <input name="account_slug" className="form-control mono" defaultValue={state.account_slug} required pattern="[a-z0-9][a-z0-9._-]*" />
              <div className="form-text">URL-friendly identifier, lowercase. Used as the default tenant.</div>
            </div>
            <div>
              <label className="form-label">Account Name</label>
              <input name="account_name" className="form-control" defaultValue={state.account_name} required />
              <div className="form-text">Human-readable name for this account/organization.</div>
            </div>
          </StepForm>
        )}

        {/* Step 4: SMTP */}
        {step === 4 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(4, {
                smtp_host: fd.get('smtp_host'),
                smtp_port: Number(fd.get('smtp_port')) || 587,
                smtp_user: fd.get('smtp_user'),
                smtp_pass: fd.get('smtp_pass'),
                smtp_from: fd.get('smtp_from'),
              });
            }}
            saving={saving}
            allowSkip
            onSkip={() => submitStep(4, {})}
          >
            <div className="row g-2">
              <div className="col-8">
                <label className="form-label">SMTP Host</label>
                <input name="smtp_host" className="form-control" defaultValue={state.smtp_host} placeholder="smtp.example.com" />
              </div>
              <div className="col-4">
                <label className="form-label">Port</label>
                <input name="smtp_port" type="number" className="form-control" defaultValue={state.smtp_port || 587} />
              </div>
            </div>
            <div>
              <label className="form-label">Username</label>
              <input name="smtp_user" className="form-control" defaultValue={state.smtp_user} />
            </div>
            <div>
              <label className="form-label">Password</label>
              <input name="smtp_pass" type="password" className="form-control" placeholder={state.smtp_pass_set ? '(saved — leave empty to keep)' : ''} />
            </div>
            <div>
              <label className="form-label">From Address</label>
              <input name="smtp_from" className="form-control" defaultValue={state.smtp_from} placeholder="noreply@example.com" />
            </div>
            <div className="form-text">Leave empty to skip SMTP (emails will be logged to console).</div>
          </StepForm>
        )}

        {/* Step 5: JWT Secret */}
        {step === 5 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(5, { jwt_secret: fd.get('jwt_secret') });
            }}
            saving={saving}
          >
            <div>
              <label className="form-label">JWT Secret</label>
              <div className="d-flex gap-2">
                <input
                  name="jwt_secret"
                  className="form-control mono flex-fill"
                  required={!state.jwt_secret_set}
                  placeholder={state.jwt_secret_set ? '(random secret generated — leave empty to keep)' : ''}
                />
                <button
                  type="button"
                  className="btn btn-outline-secondary text-nowrap"
                  onClick={() => {
                    const el = document.querySelector<HTMLInputElement>('[name=jwt_secret]');
                    if (el) el.value = generateSecret();
                  }}
                >
                  Generate
                </button>
              </div>
              <div className="form-text">Used to sign authentication tokens. Generate a strong random value for production.</div>
            </div>
          </StepForm>
        )}

        {/* Step 6: MQTT TLS */}
        {step === 6 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(6, {
                mqtt_tls_cert: fd.get('mqtt_tls_cert'),
                mqtt_tls_key: fd.get('mqtt_tls_key'),
                mqtt_tls_ca: fd.get('mqtt_tls_ca'),
              });
            }}
            saving={saving}
            allowSkip
            onSkip={() => submitStep(6, {})}
          >
            <div>
              <label className="form-label">TLS Certificate Path</label>
              <input name="mqtt_tls_cert" className="form-control mono" defaultValue={state.mqtt_tls_cert} placeholder="/opt/iot-platform/etc/certs/mqtt/cert.pem" />
            </div>
            <div>
              <label className="form-label">TLS Key Path</label>
              <input name="mqtt_tls_key" className="form-control mono" defaultValue={state.mqtt_tls_key} placeholder="/opt/iot-platform/etc/certs/mqtt/key.pem" />
            </div>
            <div>
              <label className="form-label">CA Certificate Path</label>
              <input name="mqtt_tls_ca" className="form-control mono" defaultValue={state.mqtt_tls_ca} placeholder="/opt/iot-platform/etc/certs/mqtt/ca.pem" />
              <div className="form-text">CA cert for mutual TLS (optional). Auto-filled when using Generate.</div>
            </div>
            <div className="d-flex gap-2 align-items-start">
              <button
                type="button"
                className="btn btn-outline-primary text-nowrap"
                disabled={generatingTLS}
                onClick={async () => {
                  setGeneratingTLS(true);
                  setError('');
                  try {
                    const host = window.location.hostname || 'localhost';
                    const res = await api<{ cert_path: string; key_path: string; ca_path: string }>('/setup/generate-tls', {
                      method: 'POST',
                      body: JSON.stringify({ host }),
                    });
                    const certInput = document.querySelector<HTMLInputElement>('[name=mqtt_tls_cert]');
                    const keyInput = document.querySelector<HTMLInputElement>('[name=mqtt_tls_key]');
                    const caInput = document.querySelector<HTMLInputElement>('[name=mqtt_tls_ca]');
                    if (certInput) certInput.value = res.cert_path;
                    if (keyInput) keyInput.value = res.key_path;
                    if (caInput) caInput.value = res.ca_path;
                  } catch (err) {
                    setError(err instanceof Error ? err.message : String(err));
                  } finally {
                    setGeneratingTLS(false);
                  }
                }}
              >
                {generatingTLS ? 'Generating…' : 'Generate self-signed'}
              </button>
              <span className="form-text mb-0">Generates a self-signed CA + server certificate (ECDSA P-256, 1 year validity).</span>
            </div>
            <div className="form-text">Leave empty to disable MQTT-over-TLS. Can be configured later via config.yml.</div>
          </StepForm>
        )}

        {/* Step 7: Public URL */}
        {step === 7 && state && (
          <StepForm
            onSubmit={(e) => {
              const fd = new FormData(e.currentTarget);
              submitStep(7, {
                public_base_url: fd.get('public_base_url'),
                broker_host_public: fd.get('broker_host_public'),
              });
            }}
            saving={saving}
          >
            <div>
              <label className="form-label">Public Base URL</label>
              <input name="public_base_url" className="form-control mono" defaultValue={state.public_base_url} required placeholder="https://iot.example.com" />
              <div className="form-text">
                The URL users and devices will use to reach this platform. Used for OTA firmware downloads, email verification links, and MQTT provisioning.
              </div>
            </div>
            <div>
              <label className="form-label">MQTT Broker Host / Domain</label>
              <input name="broker_host_public" className="form-control mono" defaultValue={state.broker_host_public} placeholder="iot.example.com" />
              <div className="form-text">
                Hostname or IP that devices use to connect to the MQTT broker (TCP, WS, TLS). Defaults to the hostname from Public Base URL if empty.
              </div>
            </div>
          </StepForm>
        )}

        {/* Step 8: Review & Apply */}
        {step === 8 && state && (
          <div>
            <div className="panel mb-3" style={{ background: '#141a22' }}>
              <table className="table table-sm mb-0" style={{ fontSize: '0.85rem' }}>
                <tbody>
                  <ReviewRow label="Database" value={`${state.database_driver} — ${state.database_dsn}`} />
                  <ReviewRow label="Admin" value={`${state.admin_login} (${state.admin_email})`} />
                  <ReviewRow label="Account" value={`${state.account_name} [${state.account_slug}]`} />
                  <ReviewRow label="SMTP" value={state.smtp_host ? `${state.smtp_host}:${state.smtp_port}` : '(not configured)'} />
                  <ReviewRow label="JWT Secret" value={state.jwt_secret_set ? '(set)' : '(not set)'} />
                  <ReviewRow label="MQTT TLS" value={state.mqtt_tls_cert ? `${state.mqtt_tls_cert}${state.mqtt_tls_ca ? ' + CA' : ''}` : '(disabled)'} />
                  <ReviewRow label="Public URL" value={state.public_base_url} />
                  <ReviewRow label="Broker Host" value={state.broker_host_public || '(not set)'} />
                </tbody>
              </table>
            </div>
            <div className="d-flex gap-2">
              <button className="btn btn-primary flex-fill" onClick={applySetup} disabled={applying}>
                {applying ? 'Applying…' : 'Apply Setup'}
              </button>
            </div>
            <p className="muted small mt-2">
              This will create the database, admin user, default account, and save configuration to config.yml. The application should be restarted after setup.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function StepForm({
  children,
  onSubmit,
  saving,
  allowSkip,
  onSkip,
}: {
  children: React.ReactNode;
  onSubmit: (e: FormEvent<HTMLFormElement>) => void;
  saving: boolean;
  allowSkip?: boolean;
  onSkip?: () => void;
}) {
  return (
    <form onSubmit={onSubmit} className="d-grid gap-2">
      {children}
      <div className="d-flex gap-2 mt-1">
        <button className="btn btn-primary flex-fill" type="submit" disabled={saving}>
          {saving ? 'Saving…' : 'Next'}
        </button>
        {allowSkip && (
          <button className="btn btn-outline-secondary" type="button" onClick={onSkip} disabled={saving}>
            Skip
          </button>
        )}
      </div>
    </form>
  );
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <tr>
      <td className="muted" style={{ width: '35%' }}>{label}</td>
      <td className="mono" style={{ wordBreak: 'break-all' }}>{value}</td>
    </tr>
  );
}
