import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useCallback, useRef, useState } from 'react';
import { useLang, useT } from '../i18n/I18nContext';
import { UserAvatar } from '../components/UserAvatar';
import { api } from '../api/client';
import { errorMsg, useToast } from '../components/Toast';

const NAV: [string, string][] = [
  ['/', 'nav.overview'],
  ['/mqtt', 'nav.mqtt'],
  ['/products', 'nav.products'],
  ['/devices', 'nav.devices'],
  ['/events', 'nav.events'],
  ['/logs', 'nav.logs'],
  ['/ota', 'nav.ota'],
  ['/docs', 'nav.docs'],
  ['/settings', 'nav.settings'],
];

function AccountSwitcher() {
  const { memberships, activeAccountId, switchAccount } = useAuth();
  const [open, setOpen] = useState(false);
  if (memberships.length <= 1) return null;
  const active = memberships.find((m) => m.account_id === activeAccountId);
  return (
    <div className="account-switcher position-relative">
      <button
        type="button"
        className="btn btn-sm btn-outline-secondary w-100 text-start d-flex justify-content-between align-items-center"
        onClick={() => setOpen(!open)}
      >
        <span className="text-truncate">{active?.account_name || active?.account_slug || 'No account'}</span>
        <span className={`ms-1 ${open ? 'rotate-180' : ''}`} style={{ transition: 'transform .15s' }}>&#9662;</span>
      </button>
      {open && (
        <div className="position-absolute bottom-100 start-0 end-0 mb-1 p-1 rounded border border-secondary bg-dark" style={{ zIndex: 1050 }}>
          {memberships.map((m) => (
            <button
              key={m.account_id}
              type="button"
              className={`btn btn-sm w-100 text-start text-truncate ${m.account_id === activeAccountId ? 'active' : ''}`}
              onClick={() => {
                setOpen(false);
                if (m.account_id !== activeAccountId) void switchAccount(m.account_id);
              }}
            >
              {m.account_name} <span className="muted small">({String(m.role)})</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function NavItems({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const { activeMembership } = useAuth();
  const items = [...NAV];
  if (activeMembership?.role === 'admin') {
    items.splice(items.length - 2, 0, ['/users', 'nav.users']);
    items.splice(items.length - 2, 0, ['/settings/account', 'nav.account']);
    items.splice(items.length - 2, 0, ['/audit', 'nav.auditLog']);
  }
  const isSuspended = activeMembership?.account_status === 'suspended';
  return (
    <>
      {isSuspended && (
        <div className="alert alert-warning py-1 px-2 small mb-2" style={{ fontSize: '0.75rem' }}>
          {t('widget.accountSuspended')}
        </div>
      )}
      <AccountSwitcher />
      <nav className="nav flex-column gap-1 flex-grow-1 mt-2">
        {items.map(([to, labelKey]) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}
            onClick={onNavigate}
          >
            {t(labelKey)}
          </NavLink>
        ))}
      </nav>
    </>
  );
}

function SuspendedBanner() {
  const t = useT();
  const { activeMembership } = useAuth();
  if (activeMembership?.account_status !== 'suspended') return null;
  return (
    <div className="alert alert-warning mb-0 text-center py-1 small" style={{ borderRadius: 0 }}>
      {t('widget.accountSuspended')}
    </div>
  );
}

function UserWidget() {
  const t = useT();
  const { lang, setLang } = useLang();
  const { user, logout, activeMembership, refreshMe } = useAuth();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const handleUpload = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = '';
    const fd = new FormData();
    fd.append('file', file);
    try {
      await api('/me/avatar', { method: 'POST', body: fd });
      await refreshMe();
      toast.success(t('widget.uploadAvatar'));
    } catch (err) {
      toast.error(errorMsg(err));
    }
  }, [refreshMe, toast, t]);

  const handleRemove = useCallback(async () => {
    try {
      await api('/me/avatar', { method: 'DELETE' });
      await refreshMe();
    } catch (err) {
      toast.error(errorMsg(err));
    }
  }, [refreshMe, toast]);

  if (!user) return null;

  return (
    <div className="user-widget position-relative">
      <button
        type="button"
        className="user-widget-btn d-flex align-items-center gap-2"
        onClick={() => setOpen(!open)}
        aria-label="User menu"
      >
        <UserAvatar user={user} size={30} />
        <span className="user-widget-name d-none d-lg-inline">{user.display_name || user.login || user.email}</span>
      </button>

      {open && (
        <>
          <div className="user-widget-backdrop" onClick={() => setOpen(false)} />
          <div className="user-widget-dropdown">
            {/* User info */}
            <div className="d-flex align-items-center gap-2 px-3 py-2 border-bottom border-secondary">
              <UserAvatar user={user} size={38} />
              <div className="text-truncate">
                <div className="fw-semibold text-truncate" style={{ fontSize: '0.9rem' }}>
                  {user.display_name || user.login || user.email}
                </div>
                <div className="small muted text-truncate">
                  {String(user.email)} · {String(activeMembership?.role || '')}
                </div>
              </div>
            </div>

            {/* Account switcher */}
            <div className="px-2 py-2 border-bottom border-secondary">
              <AccountSwitcher />
            </div>

            {/* Language */}
            <div className="px-3 py-2 border-bottom border-secondary">
              <div className="small muted mb-1">{t('widget.language')}</div>
              <div className="btn-group btn-group-sm w-100">
                <button
                  type="button"
                  className={`btn ${lang === 'en' ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setLang('en')}
                >
                  EN
                </button>
                <button
                  type="button"
                  className={`btn ${lang === 'ru' ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setLang('ru')}
                >
                  RU
                </button>
              </div>
            </div>

            {/* Avatar */}
            <div className="px-3 py-2 border-bottom border-secondary">
              <button
                type="button"
                className="btn btn-sm btn-outline-secondary w-100 text-start mb-1"
                onClick={() => fileRef.current?.click()}
              >
                {t('widget.uploadAvatar')}
              </button>
              <input ref={fileRef} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={handleUpload} />
              {user.avatar_url && (
                <button
                  type="button"
                  className="btn btn-sm btn-outline-danger w-100 text-start"
                  onClick={handleRemove}
                >
                  {t('widget.removeAvatar')}
                </button>
              )}
            </div>

            {/* Sign out */}
            <div className="px-3 py-2">
              <button
                type="button"
                className="btn btn-sm btn-outline-danger w-100"
                onClick={() => {
                  setOpen(false);
                  logout();
                }}
              >
                {t('common.signOut')}
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

export function AppShell() {
  return (
    <div className="app-shell d-flex">
      <aside className="sidebar d-none d-md-flex flex-column p-3">
        <div className="brand mb-3">
          <img className="brand-mark" src="/favicon.svg" alt="" />
          Cogitor <span>IoT</span>
        </div>
        <NavItems />
      </aside>

      <div className="flex-grow-1 d-flex flex-column min-vh-100">
        {/* Desktop top bar */}
        <header className="top-bar d-none d-md-flex align-items-center justify-content-end px-3 py-2 border-bottom border-secondary">
          <UserWidget />
        </header>

        {/* Mobile header */}
        <header className="d-md-none border-bottom border-secondary px-3 py-2 d-flex align-items-center gap-2">
          <button
            className="btn btn-outline-secondary btn-sm p-1"
            type="button"
            data-bs-toggle="offcanvas"
            data-bs-target="#mobileNav"
            aria-controls="mobileNav"
            aria-label="Open navigation"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <line x1="3" y1="6" x2="21" y2="6" />
              <line x1="3" y1="12" x2="21" y2="12" />
              <line x1="3" y1="18" x2="21" y2="18" />
            </svg>
          </button>
          <div className="brand flex-grow-1">
            <img className="brand-mark" src="/favicon.svg" alt="" />
            Cogitor <span>IoT</span>
          </div>
          <UserWidget />
        </header>

        <SuspendedBanner />

        <main className="main-pad flex-grow-1 p-3 p-md-4">
          <Outlet />
        </main>
      </div>

      <div className="offcanvas offcanvas-start text-bg-dark" tabIndex={-1} id="mobileNav">
        <div className="offcanvas-header">
          <h5 className="offcanvas-title brand">
            <img className="brand-mark" src="/favicon.svg" alt="" />
            Cogitor <span>IoT</span>
          </h5>
          <button type="button" className="btn-close btn-close-white" data-bs-dismiss="offcanvas" aria-label="Close" />
        </div>
        <div className="offcanvas-body d-flex flex-column">
          <NavItems onNavigate={() => {
            const el = document.getElementById('mobileNav');
            const btn = el?.querySelector('[data-bs-dismiss="offcanvas"]') as HTMLButtonElement | null;
            btn?.click();
          }} />
        </div>
      </div>
    </div>
  );
}
