import { lazy as reactLazy, Suspense, type ComponentType } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { useAuth } from './auth/AuthContext';
import { AppShell } from './layout/AppShell';

const RELOAD_KEY = 'chunk-reload-at';

// After a deploy, an open tab still references chunk hashes that no longer exist.
// Reload once to fetch the new index.html; the timestamp guard prevents a reload loop.
function lazy<T extends ComponentType<object>>(load: () => Promise<{ default: T }>) {
  return reactLazy(() =>
    load().then(
      (m) => {
        sessionStorage.removeItem(RELOAD_KEY);
        return m;
      },
      (err) => {
        const last = Number(sessionStorage.getItem(RELOAD_KEY) || 0);
        if (Date.now() - last > 10_000) {
          sessionStorage.setItem(RELOAD_KEY, String(Date.now()));
          location.reload();
          return new Promise<never>(() => {});
        }
        throw err;
      },
    ),
  );
}

const OverviewPage = lazy(() => import('./pages/OverviewPage').then((m) => ({ default: m.OverviewPage })));
const MonitoringPage = lazy(() => import('./pages/MonitoringPage').then((m) => ({ default: m.MonitoringPage })));
const MqttPage = lazy(() => import('./pages/MqttPage').then((m) => ({ default: m.MqttPage })));
const DevicesPage = lazy(() => import('./pages/DevicesPage').then((m) => ({ default: m.DevicesPage })));
const DeviceDetailPage = lazy(() => import('./pages/DeviceDetailPage').then((m) => ({ default: m.DeviceDetailPage })));
const ProductsPage = lazy(() => import('./pages/ProductsPage').then((m) => ({ default: m.ProductsPage })));
const ProductDetailPage = lazy(() => import('./pages/ProductDetailPage').then((m) => ({ default: m.ProductDetailPage })));
const EventsPage = lazy(() => import('./pages/EventsPage').then((m) => ({ default: m.EventsPage })));
const LogsPage = lazy(() => import('./pages/LogsPage').then((m) => ({ default: m.LogsPage })));
const OtaPage = lazy(() => import('./pages/OtaPage').then((m) => ({ default: m.OtaPage })));
const AuditLogPage = lazy(() => import('./pages/AuditLogPage').then((m) => ({ default: m.AuditLogPage })));
const DocsPage = lazy(() => import('./pages/DocsPage').then((m) => ({ default: m.DocsPage })));
const SettingsPage = lazy(() => import('./pages/SettingsPage').then((m) => ({ default: m.SettingsPage })));
const AccountSettingsPage = lazy(() => import('./pages/AccountSettingsPage').then((m) => ({ default: m.AccountSettingsPage })));
const UsersPage = lazy(() => import('./pages/UsersPage').then((m) => ({ default: m.UsersPage })));
const LoginPage = lazy(() => import('./pages/LoginPage').then((m) => ({ default: m.LoginPage })));
const SetupPage = lazy(() => import('./pages/SetupPage').then((m) => ({ default: m.SetupPage })));
const InviteAcceptPage = lazy(() => import('./pages/InviteAcceptPage').then((m) => ({ default: m.InviteAcceptPage })));

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token, ready, setupNeeded } = useAuth();
  if (!ready) return <div className="p-4 muted">Loading…</div>;
  if (setupNeeded) return <Navigate to="/setup" replace />;
  if (!token) return <Navigate to="/login" replace />;
  return children;
}

function PageLoader() {
  return <div className="p-4 muted">Loading…</div>;
}

export default function App() {
  return (
    <Suspense fallback={<PageLoader />}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/invite/:token" element={<InviteAcceptPage />} />
        <Route
          element={
            <RequireAuth>
              <AppShell />
            </RequireAuth>
          }
        >
          <Route path="/" element={<OverviewPage />} />
          <Route path="/monitoring" element={<MonitoringPage />} />
          <Route path="/mqtt" element={<MqttPage />} />
          <Route path="/devices" element={<DevicesPage />} />
          <Route path="/devices/:id" element={<DeviceDetailPage />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/products/:id" element={<ProductDetailPage />} />
          <Route path="/events" element={<EventsPage />} />
          <Route path="/logs" element={<LogsPage />} />
          <Route path="/ota" element={<OtaPage />} />
          <Route path="/audit" element={<AuditLogPage />} />
          <Route path="/docs" element={<DocsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/settings/account" element={<AccountSettingsPage />} />
          <Route path="/users" element={<UsersPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  );
}
