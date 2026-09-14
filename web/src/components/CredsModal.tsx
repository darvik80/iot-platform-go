import { useEffect, useMemo, useState } from 'react';
import type { DeviceCredentials } from '../types';
import { useToast } from './Toast';

const WIFI_KEY = 'cogitor.wifi';

function loadWifi() {
  try {
    const v = JSON.parse(localStorage.getItem(WIFI_KEY) || '{}') as { ssid?: string; password?: string };
    return { ssid: String(v.ssid || ''), password: String(v.password || '') };
  } catch {
    return { ssid: '', password: '' };
  }
}

export function CredsModal({
  creds,
  onClose,
}: {
  creds: DeviceCredentials;
  onClose: () => void;
}) {
  const toast = useToast();
  const user = creds.username || creds.client_id;
  const [wifi, setWifi] = useState(loadWifi);
  const [copied, setCopied] = useState('');

  useEffect(() => {
    localStorage.setItem(WIFI_KEY, JSON.stringify(wifi));
  }, [wifi]);

  const provision = useMemo(() => {
    const base = creds.provision?.cogitor;
    return {
      wifi: { ssid: wifi.ssid, password: wifi.password },
      cogitor: {
        product: base?.product || creds.product_id,
        device: base?.device || creds.device_id,
        host: base?.host || creds.broker_host,
        port: base?.port ?? creds.broker_port ?? 0,
        ws_url: base?.ws_url || creds.broker_ws_url || undefined,
        ws_port: base?.ws_port ?? creds.broker_ws_port ?? undefined,
        token: base?.token || creds.access_token,
        tls: base?.tls ?? false,
        topic_short: base?.topic_short ?? creds.topic_style !== 'full',
      },
    };
  }, [creds, wifi]);

  const jsonText = useMemo(() => JSON.stringify(provision, null, 2), [provision]);

  const copy = async (label: string, text: string) => {
    let ok = false;
    // Try modern clipboard API first
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        ok = true;
      } catch {
        // Fall through to legacy method
      }
    }
    // Legacy fallback for non-secure contexts
    if (!ok) {
      try {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.left = '-9999px';
        ta.style.top = '-9999px';
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        ok = document.execCommand('copy');
        document.body.removeChild(ta);
      } catch {
        // Last resort: show in toast
      }
    }
    if (ok) {
      setCopied(label);
      setTimeout(() => setCopied(''), 1500);
    } else {
      setCopied('');
      toast.info(text);
    }
  };

  const snippetText = useMemo(
    () =>
      `broker:      mqtt://${creds.broker_host}:${creds.broker_port}\nwebsocket:  ${creds.broker_ws_url || '(off)'}\nclient_id:   ${creds.client_id}\nusername:    ${user}\npassword:    ${creds.access_token}\ntopics:      auto (v1/me/… or iot/v1/… — detected on connect)\n\n${creds.snippet || ''}`,
    [creds, user],
  );

  const download = () => {
    const blob = new Blob([jsonText], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `${creds.product_id}-${creds.device_id}.json`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return (
    <>
      <div className="modal-backdrop fade show" />
      <div className="modal fade show d-block" tabIndex={-1} role="dialog">
        <div className="modal-dialog modal-dialog-centered modal-lg">
          <div className="modal-content">
            <div className="modal-header">
              <h5 className="modal-title">Device credentials</h5>
              <button type="button" className="btn-close btn-close-white" onClick={onClose} />
            </div>
            <div className="modal-body">
              <p className="text-danger small mb-2">Shown once — copy now. Token cannot be retrieved later.</p>
              <p className="muted small">
                MQTT: <span className="mono">username = {'{product}.{device}'}</span>,{' '}
                <span className="mono">password = access_token</span>
              </p>

              <div className="row g-2 mb-3">
                <div className="col-md-6">
                  <label className="form-label small mb-1">Wi‑Fi SSID</label>
                  <input
                    className="form-control form-control-sm"
                    placeholder="home"
                    value={wifi.ssid}
                    onChange={(e) => setWifi((w) => ({ ...w, ssid: e.target.value }))}
                  />
                </div>
                <div className="col-md-6">
                  <label className="form-label small mb-1">Wi‑Fi password</label>
                  <input
                    className="form-control form-control-sm"
                    placeholder="secret"
                    value={wifi.password}
                    onChange={(e) => setWifi((w) => ({ ...w, password: e.target.value }))}
                  />
                </div>
              </div>
              <p className="muted small mb-2">Wi‑Fi is only for the provisioning file — not stored on the platform.</p>

              <pre className="bg-dark border border-secondary rounded p-2 small mono mb-2" style={{ maxHeight: 280, overflow: 'auto' }}>
                {jsonText}
              </pre>

              <div className="d-flex flex-wrap gap-2 mb-3">
                <button type="button" className="btn btn-primary btn-sm" onClick={() => void copy('json', jsonText)}>
                  {copied === 'json' ? 'Copied JSON' : 'Copy JSON'}
                </button>
                <button type="button" className="btn btn-outline-secondary btn-sm" onClick={download}>
                  Download .json
                </button>
                <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => void copy('token', creds.access_token)}>
                  {copied === 'token' ? 'Copied token' : 'Copy token'}
                </button>
              </div>

              <details>
                <summary className="small muted">MQTT / snippet</summary>
                <pre className="bg-dark border border-secondary rounded p-2 small mono mt-2 mb-2">{snippetText}</pre>
                <button type="button" className="btn btn-outline-secondary btn-sm" onClick={() => void copy('snippet', snippetText)}>
                  {copied === 'snippet' ? 'Copied snippet' : 'Copy snippet'}
                </button>
              </details>
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
