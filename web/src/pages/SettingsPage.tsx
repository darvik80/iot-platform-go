import { useAuth } from '../auth/AuthContext';

export function SettingsPage() {
  const { settings } = useAuth();
  const s = settings || ({} as NonNullable<typeof settings>);
  return (
    <div>
      <h1 className="h3 mb-3">Settings</h1>
      <div className="panel mono small">
        <div>Platform: {s.platform_name}</div>
        <div>Retention: {s.telemetry_retention_days} days</div>
        <div>RPC timeout: {s.rpc_default_timeout_ms} ms</div>
        <div>Broker: {s.broker_host}:1883</div>
        <div>Rules bus: {String(s.rules_bus_enabled)}</div>
        <div>Node-RED: {s.nodered_url}</div>
        <div>Public URL: {s.public_base_url || '—'}</div>
        <div>Registration: {String(s.register_enabled)}</div>
        <div>Email verify: {String(s.require_email_verify)}</div>
        <div>Google OAuth: {String(s.google_enabled)}</div>
      </div>
    </div>
  );
}
