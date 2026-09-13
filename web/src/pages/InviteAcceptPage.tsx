import { useEffect, useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import type { InvitationInfo } from '../types';

export function InviteAcceptPage() {
  const { token } = useParams<{ token: string }>();
  const { token: authToken, switchAccount, activeAccountId } = useAuth();
  const navigate = useNavigate();
  const [info, setInfo] = useState<InvitationInfo | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const redirectUrl = token ? `/login?redirect=${encodeURIComponent(`/invite/${token}`)}` : '/login';
  const registerUrl = token ? `/login?mode=register&redirect=${encodeURIComponent(`/invite/${token}`)}` : '/login?mode=register';

  useEffect(() => {
    if (!token) return;
    api<InvitationInfo>(`/invitations/${token}`)
      .then(setInfo)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [token]);

  const handleAccept = async () => {
    if (!token) return;
    setBusy(true);
    try {
      const m = await api<{ account_id: number }>(`/invitations/${token}/accept`, { method: 'POST' });
      // Switch to the newly joined account
      if (m.account_id && m.account_id !== activeAccountId) {
        await switchAccount(m.account_id);
      }
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  if (error) {
    return (
      <div className="d-flex align-items-center justify-content-center min-vh-100">
        <div className="panel text-center p-4" style={{ maxWidth: 400 }}>
          <h1 className="h4 mb-3">Invitation Error</h1>
          <p className="text-danger">{error}</p>
          {!authToken && <Link to={redirectUrl} className="btn btn-primary btn-sm">Sign in</Link>}
        </div>
      </div>
    );
  }

  if (!info) {
    return <div className="d-flex align-items-center justify-content-center min-vh-100"><div className="muted">Loading…</div></div>;
  }

  const isExpired = info.status === 'expired';
  const isAccepted = info.status === 'accepted';

  return (
    <div className="d-flex align-items-center justify-content-center min-vh-100">
      <div className="panel text-center p-4" style={{ maxWidth: 420 }}>
        <h1 className="h4 mb-3">Account Invitation</h1>
        <p>You have been invited to join <strong>{info.account_name}</strong> as <strong>{info.role}</strong>.</p>
        <p className="muted small">Email: {info.email}</p>

        {isExpired && <p className="text-danger">This invitation has expired.</p>}
        {isAccepted && <p className="text-success">This invitation has already been accepted.</p>}

        {!isExpired && !isAccepted && authToken && (
          <button type="button" className="btn btn-primary" disabled={busy} onClick={handleAccept}>
            {busy ? 'Accepting…' : 'Accept Invitation'}
          </button>
        )}

        {!isExpired && !isAccepted && !authToken && (
          <div>
            <p className="small muted">Sign in or register to accept this invitation.</p>
            <Link to={redirectUrl} className="btn btn-primary btn-sm me-2">Sign in</Link>
            <Link to={registerUrl} className="btn btn-outline-secondary btn-sm">Register</Link>
          </div>
        )}
      </div>
    </div>
  );
}
