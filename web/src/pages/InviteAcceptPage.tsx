import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { useT } from '../i18n/I18nContext';
import type { InvitationInfo } from '../types';

type View =
  | 'loading'
  | 'loadError'
  | 'expired'
  | 'accepted'
  | 'accept'
  | 'mismatch'
  | 'choose'
  | 'chooseExists'
  | 'register';

export function InviteAcceptPage() {
  const { token } = useParams<{ token: string }>();
  const { token: authToken, user, activeAccountId, switchAccount, logout, setSession } = useAuth();
  const t = useT();
  const navigate = useNavigate();

  const [info, setInfo] = useState<InvitationInfo | null>(null);
  const [loadError, setLoadError] = useState('');
  const [actionError, setActionError] = useState('');
  const [mode, setMode] = useState<'choose' | 'register'>('choose');
  const [accountExists, setAccountExists] = useState(false);
  const [busy, setBusy] = useState(false);

  const redirectUrl = token ? `/login?redirect=${encodeURIComponent(`/invite/${token}`)}` : '/login';

  useEffect(() => {
    if (!token) return;
    api<InvitationInfo>(`/invitations/${token}`)
      .then(setInfo)
      .catch((e) => setLoadError(e instanceof Error ? e.message : String(e)));
  }, [token]);

  useEffect(() => {
    if (!authToken) {
      setAccountExists(false);
      setActionError('');
    }
  }, [authToken]);

  const handleAccept = async () => {
    if (!token) return;
    setBusy(true);
    setActionError('');
    try {
      const m = await api<{ account_id: number }>(`/invitations/${token}/accept`, { method: 'POST' });
      if (m.account_id && m.account_id !== activeAccountId) {
        await switchAccount(m.account_id);
      }
      navigate('/');
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleRegister = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    setActionError('');
    const fd = new FormData(e.currentTarget);
    try {
      const res = await api<{ token: string }>(`/invitations/${token}/register`, {
        method: 'POST',
        body: JSON.stringify({
          login: fd.get('login'),
          password: fd.get('password'),
          display_name: fd.get('display_name'),
        }),
      });
      await setSession(res.token);
      navigate('/');
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.toLowerCase().includes('already exists')) {
        setActionError(t('invite.accountExists'));
        setAccountExists(true);
        setMode('choose');
      } else {
        setActionError(msg);
      }
    } finally {
      setBusy(false);
    }
  };

  const emailMatches =
    !!user && !!info && user.email?.trim().toLowerCase() === info.email.trim().toLowerCase();

  const view: View = (() => {
    if (loadError) return 'loadError';
    if (!info) return 'loading';
    if (info.status === 'expired') return 'expired';
    if (info.status === 'accepted') return 'accepted';
    if (authToken && !emailMatches) return 'mismatch';
    if (authToken && emailMatches) return 'accept';
    if (!authToken && accountExists) return 'chooseExists';
    if (!authToken && mode === 'register') return 'register';
    return 'choose';
  })();

  const renderHeader = () => (
    <>
      <h1 className="h4 mb-3">{t('invite.title')}</h1>
      {info && (
        <>
          <p>{t('invite.invitedAs', { account: info.account_name, role: info.role })}</p>
          <p className="muted small">{t('common.email')}: {info.email}</p>
        </>
      )}
    </>
  );

  if (view === 'loading') {
    return (
      <div className="d-flex align-items-center justify-content-center min-vh-100">
        <div className="muted">{t('common.loading')}</div>
      </div>
    );
  }

  if (view === 'loadError') {
    return (
      <div className="d-flex align-items-center justify-content-center min-vh-100">
        <div className="panel text-center p-4" style={{ maxWidth: 400 }}>
          <h1 className="h4 mb-3">Invitation Error</h1>
          <p className="text-danger">{loadError}</p>
          {!authToken && <Link to={redirectUrl} className="btn btn-primary btn-sm">{t('common.signIn')}</Link>}
        </div>
      </div>
    );
  }

  return (
    <div className="d-flex align-items-center justify-content-center min-vh-100">
      <div className="panel text-center p-4" style={{ maxWidth: 420 }}>
        {renderHeader()}

        {view === 'expired' && <p className="text-danger">{t('invite.expired')}</p>}
        {view === 'accepted' && <p className="text-success">{t('invite.accepted')}</p>}

        {view === 'mismatch' && (
          <div>
            <div className="alert alert-warning py-2 px-3 small text-start">
              {t('invite.mismatch', { current: user?.email, invited: info!.email })}
            </div>
            <button type="button" className="btn btn-outline-secondary" onClick={logout}>
              {t('invite.signOut')}
            </button>
          </div>
        )}

        {view === 'accept' && (
          <div>
            {actionError && <div className="alert alert-danger py-2 px-3 small mb-3">{actionError}</div>}
            <button type="button" className="btn btn-primary" disabled={busy} onClick={handleAccept}>
              {busy ? t('invite.accepting') : t('invite.accept')}
            </button>
          </div>
        )}

        {view === 'choose' && (
          <div>
            <p className="small muted">{t('invite.signInToAccept')}</p>
            <Link to={redirectUrl} className="btn btn-primary btn-sm me-2">{t('common.signIn')}</Link>
            <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => setMode('register')}>
              {t('invite.createAccount')}
            </button>
          </div>
        )}

        {view === 'chooseExists' && (
          <div>
            <div className="alert alert-danger py-2 px-3 small text-start mb-3">{actionError}</div>
            <p className="small muted">{t('invite.signInToAccept')}</p>
            <Link to={redirectUrl} className="btn btn-primary">{t('common.signIn')}</Link>
          </div>
        )}

        {view === 'register' && (
          <form onSubmit={handleRegister} className="d-grid gap-2 text-start mt-3">
            {actionError && <div className="alert alert-danger py-2 px-3 small">{actionError}</div>}
            <p className="small muted mb-0">{t('invite.createFor', { email: info!.email })}</p>
            <div>
              <label className="form-label">{t('common.login')}</label>
              <input name="login" className="form-control" required pattern="[a-zA-Z][a-zA-Z0-9._-]{2,31}" placeholder="jdoe" autoFocus />
            </div>
            <div>
              <label className="form-label">{t('common.displayName')}</label>
              <input name="display_name" className="form-control" placeholder={t('common.optional')} />
            </div>
            <div>
              <label className="form-label">{t('common.password')}</label>
              <input name="password" type="password" className="form-control" minLength={8} required />
              <div className="form-text">{t('invite.passwordHint')}</div>
            </div>
            <button className="btn btn-primary" type="submit" disabled={busy}>
              {busy ? t('invite.accepting') : t('invite.accept')}
            </button>
            <a href="#" onClick={(e) => { e.preventDefault(); setMode('choose'); setActionError(''); setAccountExists(false); }}>
              {t('invite.back')}
            </a>
          </form>
        )}
      </div>
    </div>
  );
}
