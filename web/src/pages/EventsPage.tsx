import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { SeverityBadge } from '../components/StatusBadge';
import type { EventRow } from '../types';

export function EventsPage() {
  const [events, setEvents] = useState<EventRow[]>([]);
  useEffect(() => {
    void api<EventRow[]>('/events?limit=100').then(setEvents).catch((e) => alert(e.message));
  }, []);
  return (
    <div>
      <h1 className="h3 mb-3">Events</h1>
      <div className="panel table-responsive">
        <table className="table table-sm">
          <thead>
            <tr>
              <th>Time</th>
              <th>Device</th>
              <th>Severity</th>
              <th>Code</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e, i) => (
              <tr key={i}>
                <td className="mono">{new Date(e.ts).toLocaleString()}</td>
                <td className="mono">{e.product_id}.{e.device_id}</td>
                <td><SeverityBadge value={e.severity} /></td>
                <td className="mono">{e.code}</td>
                <td>{e.message || ''}</td>
              </tr>
            ))}
            {!events.length && <tr><td colSpan={5} className="muted">No events</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
