import type { DeviceCredentials } from '../types';

export function CredsModal({
  creds,
  onClose,
}: {
  creds: DeviceCredentials;
  onClose: () => void;
}) {
  const user = creds.username || creds.client_id;
  return (
    <>
      <div className="modal-backdrop fade show" />
      <div className="modal fade show d-block" tabIndex={-1} role="dialog">
        <div className="modal-dialog modal-dialog-centered">
          <div className="modal-content">
            <div className="modal-header">
              <h5 className="modal-title">Device credentials</h5>
              <button type="button" className="btn-close btn-close-white" onClick={onClose} />
            </div>
            <div className="modal-body">
              <p className="text-danger small">Shown once — copy now. Token cannot be retrieved later.</p>
              <p className="muted small">
                RabbitMQ MQTT: <span className="mono">username = {'{product}.{device}'}</span>,{' '}
                <span className="mono">password = access_token</span>
              </p>
              <pre className="bg-dark border border-secondary rounded p-2 small mono mb-0">{`broker:      mqtt://${creds.broker_host}:${creds.broker_port}
client_id:   ${creds.client_id}
username:    ${user}
password:    ${creds.access_token}
topic_style: ${creds.topic_style}

${creds.snippet || ''}`}</pre>
            </div>
            <div className="modal-footer">
              <button type="button" className="btn btn-primary" onClick={onClose}>
                Done
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
