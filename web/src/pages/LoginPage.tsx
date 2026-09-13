import { useState, useEffect, type FormEvent } from 'react';
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { useToast } from '../components/Toast';
import { useT } from '../i18n/I18nContext';

export function LoginPage() {
  const { token, ready, login, authConfig, setupNeeded } = useAuth();
  const toast = useToast();
  const t = useT();
  const [searchParams] = useSearchParams();
  const redirect = searchParams.get('redirect') || '';
  const [mode, setMode] = useState<'login' | 'register'>(
    searchParams.get('mode') === 'register' ? 'register' : 'login',
  );
  const [error, setError] = useState('');
  const [ok, setOk] = useState('');
  const navigate = useNavigate();

  // Sync mode if URL changes (e.g. coming from invite link)
  useEffect(() => {
    if (searchParams.get('mode') === 'register') setMode('register');
  }, [searchParams]);

  if (!ready) return null;
  if (setupNeeded) return <Navigate to="/setup" replace />;
  if (token) return <Navigate to={redirect || '/'} replace />;

  const onLogin = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError('');
    const fd = new FormData(e.currentTarget);
    try {
      await login(String(fd.get('email') || ''), String(fd.get('password') || ''));
      navigate(redirect || '/');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const onRegister = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError('');
    setOk('');
    const fd = new FormData(e.currentTarget);
    try {
      const res = await api<{ message?: string }>('/auth/register', {
        method: 'POST',
        body: JSON.stringify({
          login: fd.get('login'),
          email: fd.get('email'),
          display_name: fd.get('display_name'),
          password: fd.get('password'),
        }),
      });
      setOk(res.message || t('auth.registered'));
      setTimeout(() => setMode('login'), 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="login-wrap">
      <div className="login-box">
        <h1 className="h3 brand">
          <img className="brand-mark" src="/favicon.svg" alt="" />
          Cogitor <span>IoT</span>
        </h1>
        <p className="muted">{mode === 'register' ? t('auth.createAccount') : t('auth.opsConsole')}</p>

        {redirect.includes('/invite/') && (
          <div className="alert alert-info py-2 px-3 small mb-3">
            {t('auth.invitationNotice')}
          </div>
        )}

        {mode === 'register' ? (
          <form onSubmit={onRegister} className="d-grid gap-2">
            <div>
              <label className="form-label">{t('common.login')}</label>
              <input name="login" className="form-control" required pattern="[a-zA-Z][a-zA-Z0-9._-]{2,31}" placeholder="jdoe" />
            </div>
            <div>
              <label className="form-label">{t('common.email')}</label>
              <input name="email" type="email" className="form-control" required />
            </div>
            <div>
              <label className="form-label">{t('common.displayName')}</label>
              <input name="display_name" className="form-control" placeholder={t('common.optional')} />
            </div>
            <div>
              <label className="form-label">{t('common.password')}</label>
              <input name="password" type="password" className="form-control" minLength={8} required />
            </div>
            <button className="btn btn-primary" type="submit">
              {t('common.register')}
            </button>
            {(error || ok) && <div className={ok ? 'text-success' : 'text-danger'}>{ok || error}</div>}
            <p className="muted small mb-0">
              {t('auth.alreadyHaveAccount')}{' '}
              <a href="#" onClick={(e) => { e.preventDefault(); setMode('login'); }}>
                {t('common.signIn')}
              </a>
            </p>
          </form>
        ) : (
          <>
            <form onSubmit={onLogin} className="d-grid gap-2">
              <div>
                <label className="form-label">{t('auth.loginOrEmail')}</label>
                <input name="email" className="form-control" defaultValue="admin" required />
              </div>
              <div>
                <label className="form-label">{t('common.password')}</label>
                <input name="password" type="password" className="form-control" defaultValue="admin" required />
              </div>
              <button className="btn btn-primary" type="submit">
                {t('common.signIn')}
              </button>
              {error && <div className="text-danger">{error}</div>}
            </form>
            <div className="mt-3">
              {authConfig?.google_enabled ? (
                <a className="btn btn-outline-secondary w-100" href="/api/v1/auth/google">
                  {t('auth.continueWithGoogle')}
                </a>
              ) : (
                <button
                  type="button"
                  className="btn btn-outline-secondary w-100"
                  onClick={() =>
                    toast.info(
                      `Google OAuth not configured. Redirect URI: ${location.origin}/api/v1/auth/google/callback — set GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET / PUBLIC_BASE_URL in .env`,
                    )
                  }
                >
                  {t('auth.continueWithGoogle')}
                </button>
              )}
            </div>
            {!authConfig?.google_enabled && (
              <p className="muted small mt-2">{t('auth.googleNotConfigured')}</p>
            )}
            {authConfig?.register_enabled && (
              <p className="muted small mt-3 mb-0">
                {t('auth.noAccount')}{' '}
                <a href="#" onClick={(e) => { e.preventDefault(); setMode('register'); }}>
                  {t('common.register')}
                </a>
              </p>
            )}
            <p className="d-none">
              <Link to="/">home</Link>
            </p>
          </>
        )}
      </div>
    </div>
  );
}
