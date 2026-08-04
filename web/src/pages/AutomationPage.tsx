import { useAuth } from '../auth/AuthContext';

export function AutomationPage() {
  const { settings } = useAuth();
  const url = settings?.nodered_url || 'http://localhost:1880';
  return (
    <div>
      <h1 className="h3 mb-3">Automation</h1>
      <div className="panel">
        <p>
          Rules / CEP are handled by <strong>Node-RED</strong>. The platform republishes normalized events and accepts
          commands on the rules bus.
        </p>
        <p className="mono small">Events: platform/v1/events/{'{product}'}/{'{device}'}/#</p>
        <p className="mono small">Commands: platform/v1/commands/{'{product}'}/{'{device}'}/rpc|attributes/desired|ota</p>
        <a className="btn btn-primary" href={url} target="_blank" rel="noreferrer">
          Open Node-RED
        </a>
        <p className="muted small mt-3 mb-0">
          MQTT user for Node-RED: <span className="mono">nodered</span>. Do not use device access tokens in flows.
        </p>
      </div>
    </div>
  );
}
