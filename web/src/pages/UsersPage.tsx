import { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import type { Role, User } from '../types';

export function UsersPage() {
  const { user } = useAuth();
  const [users, setUsers] = useState<User[]>([]);

  const load = () => api<User[]>('/users').then(setUsers);

  useEffect(() => {
    if (user?.role === 'admin') void load().catch((e) => alert(e.message));
  }, [user]);

  if (user?.role !== 'admin') return <Navigate to="/" replace />;

  return (
    <div>
      <h1 className="h3 mb-3">Users</h1>
      <div className="panel table-responsive">
        <table className="table table-sm align-middle">
          <thead>
            <tr>
              <th>Login</th>
              <th>Email</th>
              <th>Role</th>
              <th>Provider</th>
              <th>Verified</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td className="mono">{u.login}</td>
                <td>{u.email}</td>
                <td>
                  <select
                    className="form-select form-select-sm"
                    value={u.role}
                    onChange={async (e) => {
                      try {
                        await api(`/users/${u.id}`, { method: 'PATCH', body: JSON.stringify({ role: e.target.value as Role }) });
                        await load();
                      } catch (err) {
                        alert(err instanceof Error ? err.message : String(err));
                      }
                    }}
                  >
                    {(['viewer', 'operator', 'admin'] as Role[]).map((r) => (
                      <option key={r} value={r}>
                        {r}
                      </option>
                    ))}
                  </select>
                </td>
                <td>{u.auth_provider || ''}</td>
                <td>{u.email_verified ? 'yes' : 'no'}</td>
                <td>
                  <span className={`badge ${u.disabled ? 'badge-offline' : 'badge-online'}`}>
                    {u.disabled ? 'disabled' : 'active'}
                  </span>
                </td>
                <td className="text-nowrap">
                  <button
                    type="button"
                    className="btn btn-outline-secondary btn-sm me-1"
                    onClick={async () => {
                      try {
                        await api(`/users/${u.id}`, { method: 'PATCH', body: JSON.stringify({ disabled: !u.disabled }) });
                        await load();
                      } catch (err) {
                        alert(err instanceof Error ? err.message : String(err));
                      }
                    }}
                  >
                    {u.disabled ? 'Enable' : 'Disable'}
                  </button>
                  <button
                    type="button"
                    className="btn btn-danger btn-sm"
                    onClick={async () => {
                      if (!confirm(`Delete user #${u.id}?`)) return;
                      try {
                        await api(`/users/${u.id}`, { method: 'DELETE' });
                        await load();
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
    </div>
  );
}
