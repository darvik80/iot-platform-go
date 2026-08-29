import { useAuth } from '../auth/AuthContext';

export function AutomationPage() {
  const { settings } = useAuth();
  const url = settings?.nodered_url || 'http://localhost:1880';
  return (
    <div className="nodered-embed d-flex flex-column">
      <div className="d-flex flex-wrap align-items-baseline justify-content-between gap-2 mb-2">
        <h1 className="h3 mb-0">Automation</h1>
        <a className="small" href={url} target="_blank" rel="noreferrer">
          Open in new tab
        </a>
      </div>
      <p className="muted small mb-2">
        Node-RED editor (MQTT user <span className="code">nodered</span>). Events{' '}
        <span className="code">platform/v1/events/…</span> → commands{' '}
        <span className="code">platform/v1/commands/…/rpc</span>.
      </p>
      <iframe
        className="nodered-frame"
        title="Node-RED"
        src={url}
        referrerPolicy="no-referrer-when-downgrade"
      />
    </div>
  );
}
