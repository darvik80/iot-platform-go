import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { SeverityBadge } from '../components/StatusBadge';
import type { LogRow } from '../types';

export function LogsPage() {
  const [logs, setLogs] = useState<LogRow[]>([]);
  useEffect(() => {
    void api<LogRow[]>('/logs?limit=200').then(setLogs).catch((e) => alert(e.message));
  }, []);
  return (
    <div>
      <h1 className="h3 mb-3">Device logs</h1>
      <div className="panel table-responsive">
        <table className="table table-sm">
          <thead>
            <tr>
              <th>Time</th>
              <th>Level</th>
              <th>Device</th>
              <th>Module</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((l, i) => (
              <tr key={i}>
                <td className="mono">{new Date(l.ts).toLocaleString()}</td>
                <td><SeverityBadge value={l.level} /></td>
                <td className="mono">{l.product_id}.{l.device_id}</td>
                <td>{l.module}</td>
                <td>{l.message}</td>
              </tr>
            ))}
            {!logs.length && <tr><td colSpan={5} className="muted">No logs</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
