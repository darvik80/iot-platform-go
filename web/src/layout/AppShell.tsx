import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';

const NAV: [string, string][] = [
  ['/', 'Overview'],
  ['/mqtt', 'MQTT'],
  ['/products', 'Products'],
  ['/devices', 'Devices'],
  ['/events', 'Events'],
  ['/logs', 'Logs'],
  ['/ota', 'OTA'],
  ['/automation', 'Automation'],
  ['/settings', 'Settings'],
];

function NavItems({ onNavigate }: { onNavigate?: () => void }) {
  const { user, logout } = useAuth();
  const items = [...NAV];
  if (user?.role === 'admin') {
    items.splice(items.length - 1, 0, ['/users', 'Users']);
  }
  return (
    <>
      <nav className="nav flex-column gap-1 flex-grow-1">
        {items.map(([to, label]) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}
            onClick={onNavigate}
          >
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="mt-3 small muted">
        {user?.login || user?.email} · {user?.role}
      </div>
      <button
        type="button"
        className="btn btn-outline-secondary btn-sm mt-2"
        onClick={() => {
          logout();
          onNavigate?.();
        }}
      >
        Sign out
      </button>
    </>
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
        <header className="d-md-none border-bottom border-secondary px-3 py-2 d-flex align-items-center gap-2">
          <button
            className="btn btn-outline-secondary btn-sm"
            type="button"
            data-bs-toggle="offcanvas"
            data-bs-target="#mobileNav"
            aria-controls="mobileNav"
          >
            Menu
          </button>
          <div className="brand flex-grow-1">
            <img className="brand-mark" src="/favicon.svg" alt="" />
            Cogitor <span>IoT</span>
          </div>
        </header>

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
