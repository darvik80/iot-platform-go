import { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { errorMsg, useToast } from '../components/Toast';
import { formatAge } from '../utils/format';
import { useT } from '../i18n/I18nContext';
import type { AccountInvitation, MemberInfo, Role } from '../types';

type Tab = 'members' | 'invitations';

export function UsersPage() {
  const { activeMembership, user, refreshMe } = useAuth();
  const toast = useToast();
  const t = useT();
  const [tab, setTab] = useState<Tab>('members');
  const [members, setMembers] = useState<MemberInfo[]>([]);
  const [invitations, setInvitations] = useState<AccountInvitation[]>([]);
  const [busy, setBusy] = useState(false);

  // Create member form
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [newLogin, setNewLogin] = useState('');
  const [newEmail, setNewEmail] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState<Role>('viewer');
  const [newDisplayName, setNewDisplayName] = useState('');

  // Invite form
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<Role>('viewer');
  const [inviteMethod, setInviteMethod] = useState('email');
  const [inviteUrl, setInviteUrl] = useState('');

  // Edit user modal
  const [editingUser, setEditingUser] = useState<MemberInfo | null>(null);
  const [editEmail, setEditEmail] = useState('');
  const [editDisplayName, setEditDisplayName] = useState('');
  const [editPassword, setEditPassword] = useState('');

  if (activeMembership?.role !== 'admin') return <Navigate to="/" replace />;

  const loadMembers = () => api<MemberInfo[]>('/members').then(setMembers).catch(console.error);
  const loadInvitations = () => api<AccountInvitation[]>('/invitations').then(setInvitations).catch(console.error);

  useEffect(() => {
    loadMembers();
    loadInvitations();
  }, []);

  // ── Member actions ──────────────────────────────────────────────────────

  const handleCreateMember = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api('/members', {
        method: 'POST',
        body: JSON.stringify({
          login: newLogin,
          email: newEmail,
          password: newPassword,
          role: newRole,
          display_name: newDisplayName,
        }),
      });
      setShowCreateForm(false);
      setNewLogin('');
      setNewEmail('');
      setNewPassword('');
      setNewRole('viewer');
      setNewDisplayName('');
      await loadMembers();
    } catch (err) {
      toast.error(errorMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const handleChangeRole = async (userId: number, role: string) => {
    try {
      await api(`/members/${userId}`, { method: 'PATCH', body: JSON.stringify({ role }) });
      await loadMembers();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  };

  const handleRemoveMember = async (userId: number) => {
    if (!confirm(t('users.removeConfirm'))) return;
    try {
      await api(`/members/${userId}`, { method: 'DELETE' });
      await loadMembers();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  };

  const handleOpenEdit = (m: MemberInfo) => {
    setEditingUser(m);
    setEditEmail(m.email);
    setEditDisplayName(m.display_name);
    setEditPassword('');
  };

  const handleUpdateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingUser) return;
    setBusy(true);
    try {
      const body: Record<string, string> = {};
      if (editEmail !== editingUser.email) body.email = editEmail;
      if (editDisplayName !== editingUser.display_name) body.display_name = editDisplayName;
      if (editPassword) body.password = editPassword;
      await api(`/users/${editingUser.user_id}`, {
        method: 'PATCH',
        body: JSON.stringify(body),
      });
      setEditingUser(null);
      await loadMembers();
      // Refresh auth context if editing yourself
      if (user?.id === editingUser.user_id) {
        await refreshMe();
      }
      toast.success('User updated');
    } catch (err) {
      toast.error(errorMsg(err));
    } finally {
      setBusy(false);
    }
  };

  // ── Invitation actions ──────────────────────────────────────────────────

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setInviteUrl('');
    try {
      const res = await api<{ invite_url: string }>('/invitations', {
        method: 'POST',
        body: JSON.stringify({
          email: inviteEmail,
          role: inviteRole,
          delivery_method: inviteMethod,
        }),
      });
      setInviteUrl(res.invite_url);
      setInviteEmail('');
      await loadInvitations();
    } catch (err) {
      toast.error(errorMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const handleResend = async (id: number) => {
    try {
      await api(`/invitations/${id}/resend`, { method: 'POST' });
      await loadInvitations();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  };

  const handleRevoke = async (id: number) => {
    if (!confirm(t('users.revokeConfirm'))) return;
    try {
      await api(`/invitations/${id}`, { method: 'DELETE' });
      await loadInvitations();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  };

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <div>
      <h1 className="h3 mb-3">{t('users.title')}</h1>

      <ul className="nav nav-tabs mb-3">
        {(['members', 'invitations'] as Tab[]).map((tabKey) => (
          <li key={tabKey} className="nav-item">
            <button
              type="button"
              className={`nav-link${tab === tabKey ? ' active' : ''}`}
              onClick={() => setTab(tabKey)}
            >
              {tabKey === 'members' ? t('users.members') : t('users.invitations')}
              {tabKey === 'invitations' && invitations.filter((i) => i.status === 'pending').length > 0 && (
                <span className="badge badge-info ms-1">{invitations.filter((i) => i.status === 'pending').length}</span>
              )}
            </button>
          </li>
        ))}
      </ul>

      {/* ── Members tab ─────────────────────────────────────────────────── */}
      {tab === 'members' && (
        <div>
          <div className="d-flex gap-2 mb-3 align-items-center">
            <button
              type="button"
              className="btn btn-primary btn-sm"
              onClick={() => setShowCreateForm(!showCreateForm)}
              title="Create a new user and add them to this account"
            >
              {t('users.createUser')}
            </button>
            <span className="help-icon" title="Create a new user account manually and add it to the current account. The user will be able to log in immediately.">
              i
            </span>
          </div>

          {showCreateForm && (
            <form className="panel d-flex flex-wrap gap-2 mb-3" onSubmit={handleCreateMember}>
              <input
                className="form-control form-control-sm"
                style={{ maxWidth: 160 }}
                placeholder="Login"
                value={newLogin}
                onChange={(e) => setNewLogin(e.target.value)}
                required
                title="Username for login (3-32 chars, letters, digits, ._-)"
              />
              <input
                className="form-control form-control-sm"
                style={{ maxWidth: 200 }}
                placeholder="Email"
                type="email"
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                required
                title="Email address for login and notifications"
              />
              <input
                className="form-control form-control-sm"
                style={{ maxWidth: 160 }}
                placeholder="Display name"
                value={newDisplayName}
                onChange={(e) => setNewDisplayName(e.target.value)}
                title="Optional display name shown in the UI"
              />
              <input
                className="form-control form-control-sm"
                style={{ maxWidth: 160 }}
                placeholder="Password"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                required
                minLength={8}
                title="Minimum 8 characters, must contain letters and digits"
              />
              <select
                className="form-select form-select-sm"
                style={{ maxWidth: 120 }}
                value={newRole}
                onChange={(e) => setNewRole(e.target.value as Role)}
                title="Role: viewer (read-only), operator (control devices), admin (full access)"
              >
                <option value="viewer">viewer</option>
                <option value="operator">operator</option>
                <option value="admin">admin</option>
              </select>
              <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
                {t('common.create')}
              </button>
              <button
                type="button"
                className="btn btn-outline-secondary btn-sm"
                onClick={() => {
                  setShowCreateForm(false);
                  setNewLogin('');
                  setNewEmail('');
                  setNewPassword('');
                  setNewRole('viewer');
                  setNewDisplayName('');
                }}
              >
                {t('common.cancel')}
              </button>
            </form>
          )}

          <div className="panel table-responsive">
            <table className="table table-sm align-middle mb-0">
              <thead>
                <tr>
                  <th>{t('common.login')}</th>
                  <th>{t('common.email')}</th>
                  <th>{t('common.displayName')}</th>
                  <th>{t('common.role')}</th>
                  <th>{t('common.status')}</th>
                  <th>{t('users.joined')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {members.length === 0 && (
                  <tr>
                    <td colSpan={7} className="muted small">
                      {t('users.noMembers')}
                    </td>
                  </tr>
                )}
                {members.map((m) => (
                  <tr key={m.user_id}>
                    <td className="mono">{m.login}</td>
                    <td>{m.email}</td>
                    <td>{m.display_name || <span className="muted">—</span>}</td>
                    <td>
                      <select
                        className="form-select form-select-sm"
                        style={{ width: 'auto' }}
                        value={m.role}
                        onChange={(e) => handleChangeRole(m.user_id, e.target.value)}
                        title="Change member role"
                      >
                        {(['viewer', 'operator', 'admin'] as const).map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <span className={`badge ${m.status === 'active' ? 'badge-online' : 'badge-offline'}`}>
                        {m.status}
                      </span>
                    </td>
                    <td className="small">{formatAge(m.joined_at)}</td>
                    <td className="text-end">
                      <button
                        type="button"
                        className="btn btn-outline-secondary btn-sm me-1"
                        onClick={() => handleOpenEdit(m)}
                        title="Edit email, display name, or password"
                      >
                        {t('common.edit')}
                      </button>
                      <button
                        type="button"
                        className="btn btn-outline-danger btn-sm"
                        onClick={() => handleRemoveMember(m.user_id)}
                        title="Remove this user from the account (does not delete the user account)"
                      >
                        {t('users.remove')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Invitations tab ─────────────────────────────────────────────── */}
      {tab === 'invitations' && (
        <div>
          <div className="d-flex align-items-center gap-2 mb-3">
            <p className="muted small mb-0">
              {t('users.inviteDescription')}
            </p>
            <span className="help-icon" title="Invitations allow users to join your account with specific roles (viewer, operator, admin)">
              i
            </span>
          </div>

          <form className="panel d-flex flex-wrap gap-2 mb-3" onSubmit={handleInvite}>
            <input
              className="form-control form-control-sm"
              style={{ maxWidth: 220 }}
              placeholder={t('users.emailAddress')}
              type="email"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
              required
              title="Email address of the user to invite"
            />
            <select
              className="form-select form-select-sm"
              style={{ maxWidth: 120 }}
              value={inviteRole}
              onChange={(e) => setInviteRole(e.target.value as Role)}
              title="Role determines what the user can do: viewer (read-only), operator (control devices), admin (full access)"
            >
              <option value="viewer">viewer</option>
              <option value="operator">operator</option>
              <option value="admin">admin</option>
            </select>
            <select
              className="form-select form-select-sm"
              style={{ maxWidth: 140 }}
              value={inviteMethod}
              onChange={(e) => setInviteMethod(e.target.value)}
              title="Choose how to deliver the invitation"
            >
              <option value="email">{t('users.sendEmail')}</option>
              <option value="link">{t('users.shareableLink')}</option>
            </select>
            <button type="submit" className="btn btn-primary btn-sm" disabled={busy} title="Send invitation">
              {t('users.invite')}
            </button>
          </form>

          {inviteUrl && (
            <div className="panel mb-3">
              <p className="small mb-1">{t('users.shareLink')}</p>
              <pre className="bg-dark border border-secondary rounded p-2 small mono user-select-all">{inviteUrl}</pre>
            </div>
          )}

          <div className="panel table-responsive">
            <table className="table table-sm align-middle mb-0">
              <thead>
                <tr>
                  <th>{t('common.email')}</th>
                  <th>{t('common.role')}</th>
                  <th>{t('common.status')}</th>
                  <th>{t('users.sent')}</th>
                  <th>{t('users.expires')}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {invitations.length === 0 && (
                  <tr>
                    <td colSpan={6} className="muted small">
                      {t('users.noInvitations')}
                    </td>
                  </tr>
                )}
                {invitations.map((inv) => (
                  <tr key={inv.id}>
                    <td>{inv.email}</td>
                    <td>{inv.role}</td>
                    <td>
                      <span
                        className={`badge ${
                          inv.status === 'pending' ? 'badge-info' : inv.status === 'accepted' ? 'badge-online' : 'badge-offline'
                        }`}
                      >
                        {inv.status}
                      </span>
                    </td>
                    <td className="small">{formatAge(inv.created_at)}</td>
                    <td className="small">{formatAge(inv.expires_at)}</td>
                    <td className="text-end text-nowrap">
                      {inv.status === 'pending' && (
                        <>
                          <button
                            type="button"
                            className="btn btn-outline-secondary btn-sm me-1"
                            onClick={() => handleResend(inv.id)}
                            title="Resend the invitation email"
                          >
                            {t('users.resend')}
                          </button>
                          <button
                            type="button"
                            className="btn btn-outline-danger btn-sm"
                            onClick={() => handleRevoke(inv.id)}
                            title="Cancel this invitation"
                          >
                            {t('users.revoke')}
                          </button>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Edit user modal ────────────────────────────────────────────── */}
      {editingUser && (
        <div className="modal-backdrop" onClick={() => setEditingUser(null)}>
          <div className="modal show d-block" tabIndex={-1} onClick={(e) => e.stopPropagation()}>
            <div className="modal-dialog">
              <div className="modal-content bg-dark border-secondary">
                <form onSubmit={handleUpdateUser}>
                  <div className="modal-header border-secondary">
                    <h5 className="modal-title">Edit user: {editingUser.login}</h5>
                    <button type="button" className="btn-close btn-close-white" onClick={() => setEditingUser(null)} />
                  </div>
                  <div className="modal-body">
                    <div className="mb-3">
                      <label className="form-label small">Email</label>
                      <input
                        type="email"
                        className="form-control bg-dark text-light border-secondary"
                        value={editEmail}
                        onChange={(e) => setEditEmail(e.target.value)}
                        required
                      />
                    </div>
                    <div className="mb-3">
                      <label className="form-label small">Display name</label>
                      <input
                        type="text"
                        className="form-control bg-dark text-light border-secondary"
                        value={editDisplayName}
                        onChange={(e) => setEditDisplayName(e.target.value)}
                      />
                    </div>
                    <div className="mb-3">
                      <label className="form-label small">New password <span className="muted">(leave empty to keep current)</span></label>
                      <input
                        type="password"
                        className="form-control bg-dark text-light border-secondary"
                        value={editPassword}
                        onChange={(e) => setEditPassword(e.target.value)}
                        minLength={8}
                        placeholder="Minimum 8 characters"
                      />
                    </div>
                  </div>
                  <div className="modal-footer border-secondary">
                    <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => setEditingUser(null)}>
                      {t('common.cancel')}
                    </button>
                    <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
                      {t('common.save')}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
