import { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { AuditEntry } from '../types';
import { formatAge } from '../utils/format';
import { useT } from '../i18n/I18nContext';

export function AuditLogPage() {
  const t = useT();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const data = await api<AuditEntry[]>('/audit-log?limit=200');
      setEntries(data);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div>
      <div className="d-flex align-items-center justify-content-between mb-3">
        <h1 className="h3 mb-0">{t('audit.title')}</h1>
        <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => void load()}>
          {t('common.refresh')}
        </button>
      </div>

      {error && <div className="alert alert-danger py-2">{error}</div>}

      {loading ? (
        <div className="muted">{t('common.loading')}</div>
      ) : (
        <div className="panel table-responsive">
          <table className="table table-sm align-middle mb-0">
            <thead>
              <tr>
                <th>{t('common.time')}</th>
                <th>{t('overview.user')}</th>
                <th>{t('audit.actor')}</th>
                <th>{t('audit.action')}</th>
                <th>{t('audit.entityType')}</th>
                <th>{t('audit.entityId')}</th>
                <th>{t('audit.details')}</th>
              </tr>
            </thead>
            <tbody>
              {entries.length === 0 && (
                <tr>
                  <td colSpan={7} className="muted small">
                    No audit entries found.
                  </td>
                </tr>
              )}
              {entries.map((e) => (
                <tr key={e.id}>
                  <td className="mono small text-nowrap">{formatAge(e.created_at)}</td>
                  <td className="small">{e.user_login || '—'}</td>
                  <td>
                    <span className={`badge ${e.actor_kind === 'user' ? 'text-bg-primary' : 'text-bg-secondary'}`}>
                      {e.actor_kind}
                    </span>
                  </td>
                  <td className="mono small">{e.action}</td>
                  <td className="small">{e.entity_type}</td>
                  <td className="mono small text-truncate" style={{ maxWidth: 160 }} title={e.entity_id}>
                    {e.entity_id}
                  </td>
                  <td className="mono small">
                    {e.detail_json ? (
                      <details>
                        <summary className="cursor-pointer text-truncate" style={{ maxWidth: 200 }}>
                          JSON
                        </summary>
                        <pre className="bg-dark border border-secondary rounded p-2 small mono mb-0 mt-1" style={{ maxHeight: 200, overflow: 'auto' }}>
                          {(() => {
                            try {
                              return JSON.stringify(JSON.parse(e.detail_json), null, 2);
                            } catch {
                              return e.detail_json;
                            }
                          })()}
                        </pre>
                      </details>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
