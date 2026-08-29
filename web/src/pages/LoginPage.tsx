import { useState, type FormEvent } from 'react';
import { Link, Navigate, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';

export function LoginPage() {
  const { token, ready, login, authConfig } = useAuth();
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [error, setError] = useState('');
  const [ok, setOk] = useState('');
  const navigate = useNavigate();

  if (!ready) return null;
  if (token) return <Navigate to="/" replace />;

  const onLogin = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError('');
    const fd = new FormData(e.currentTarget);
    try {
      await login(String(fd.get('email') || ''), String(fd.get('password') || ''));
      navigate('/');
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
      setOk(res.message || 'Registered. Check email to verify, then sign in.');
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
        <p className="muted">{mode === 'register' ? 'Create an account' : 'Ops console — sign in'}</p>

        {mode === 'register' ? (
          <form onSubmit={onRegister} className="d-grid gap-2">
            <div>
              <label className="form-label">Login</label>
              <input name="login" className="form-control" required pattern="[a-zA-Z][a-zA-Z0-9._-]{2,31}" placeholder="jdoe" />
            </div>
            <div>
              <label className="form-label">Email</label>
              <input name="email" type="email" className="form-control" required />
            </div>
            <div>
              <label className="form-label">Display name</label>
              <input name="display_name" className="form-control" placeholder="optional" />
            </div>
            <div>
              <label className="form-label">Password</label>
              <input name="password" type="password" className="form-control" minLength={8} required />
            </div>
            <button className="btn btn-primary" type="submit">
              Register
            </button>
            {(error || ok) && <div className={ok ? 'text-success' : 'text-danger'}>{ok || error}</div>}
            <p className="muted small mb-0">
              Already have an account?{' '}
              <a href="#" onClick={(e) => { e.preventDefault(); setMode('login'); }}>
                Sign in
              </a>
            </p>
          </form>
        ) : (
          <>
            <form onSubmit={onLogin} className="d-grid gap-2">
              <div>
                <label className="form-label">Login or email</label>
                <input name="email" className="form-control" defaultValue="admin" required />
              </div>
              <div>
                <label className="form-label">Password</label>
                <input name="password" type="password" className="form-control" defaultValue="admin" required />
              </div>
              <button className="btn btn-primary" type="submit">
                Sign in
              </button>
              {error && <div className="text-danger">{error}</div>}
            </form>
            <div className="mt-3">
              {authConfig?.google_enabled ? (
                <a className="btn btn-outline-secondary w-100" href="/api/v1/auth/google">
                  Continue with Google
                </a>
              ) : (
                <button
                  type="button"
                  className="btn btn-outline-secondary w-100"
                  onClick={() =>
                    alert(
                      `Google OAuth is not configured.\n\nRedirect URI: ${location.origin}/api/v1/auth/google/callback\nSet GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET / PUBLIC_BASE_URL in .env`,
                    )
                  }
                >
                  Continue with Google
                </button>
              )}
            </div>
            {!authConfig?.google_enabled && (
              <p className="muted small mt-2">Google OAuth not configured.</p>
            )}
            {authConfig?.register_enabled && (
              <p className="muted small mt-3 mb-0">
                No account?{' '}
                <a href="#" onClick={(e) => { e.preventDefault(); setMode('register'); }}>
                  Register
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
