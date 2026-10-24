import { useState } from 'react';
import { createWidget, MAX_WIDGETS, moveWidget, newWidgetID, type MonitoringDashboard, type Widget } from '../../api/monitoring';
import { useT } from '../../i18n/I18nContext';
import type { Device } from '../../types';
import { isCancelled, type MonitoringRequest, useMonitoringResource } from './hooks';
import { Editor, Field, FormActions, ResourceState } from './Shared';
import { WidgetQuery } from './WidgetView';
import { WidgetWizard } from './WidgetWizard';

export function Dashboards({ request, writable, devices, refresh }: { request: MonitoringRequest; writable: boolean; devices: Device[]; refresh: number }) {
  const t = useT();
  const list = useMonitoringResource<MonitoringDashboard[]>(request, '/monitoring/dashboards', refresh);
  const [selected, setSelected] = useState<number | null>(null);
  const [creating, setCreating] = useState(false);
  const dashboards = list.data || [];
  const id = dashboards.find(d => d.id === selected)?.id ?? dashboards[0]?.id;
  const replace = (dashboard: MonitoringDashboard) => list.setData(current => (current || []).map(d => d.id === dashboard.id ? dashboard : d));
  return <>
    <div className="d-flex align-items-end flex-wrap gap-2 mb-3">
      {!!dashboards.length && <div className="flex-grow-1"><label className="form-label" htmlFor="monitoring-dashboard">{t('monitoring.dashboard')}</label><select id="monitoring-dashboard" className="form-select" value={id} onChange={e => setSelected(Number(e.target.value))}>{dashboards.map(d => <option key={d.id} value={d.id}>{d.name}</option>)}</select></div>}
      {writable && <button className="btn btn-primary" onClick={() => setCreating(true)}>{t('monitoring.createDashboard')}</button>}
    </div>
    <ResourceState loading={list.loading} error={list.error} retry={list.reload} />
    {!list.loading && !list.error && !dashboards.length && <div className="panel text-center py-5"><h2 className="h5">{t('monitoring.noDashboards')}</h2><p className="muted mb-0">{t(writable ? 'monitoring.createDashboardHint' : 'monitoring.viewerDashboardHint')}</p></div>}
    {id != null && <DashboardDetail key={id} id={id} request={request} devices={devices} writable={writable} refresh={refresh} onChange={replace} onDelete={() => { list.setData(current => (current || []).filter(d => d.id !== id)); setSelected(current => current === id ? null : current); }} />}
    {creating && writable && <NameEditor title={t('monitoring.createDashboard')} initial="" onClose={() => setCreating(false)} onSave={async name => {
      const dashboard = await request<MonitoringDashboard>('/monitoring/dashboards', { method: 'POST', body: JSON.stringify({ name, widgets: [] }) });
      list.setData(current => [...(current || []), dashboard]); setSelected(dashboard.id);
    }} />}
  </>;
}
function DashboardDetail({ id, request, devices, writable, refresh, onChange, onDelete }: { id: number; request: MonitoringRequest; devices: Device[]; writable: boolean; refresh: number; onChange: (dashboard: MonitoringDashboard) => void; onDelete: () => void }) {
  const t = useT();
  const resource = useMonitoringResource<MonitoringDashboard>(request, `/monitoring/dashboards/${id}`, refresh);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState<{ widget: Widget; dashboard: MonitoringDashboard } | null>(null);
  const [renaming, setRenaming] = useState<MonitoringDashboard | null>(null);
  const dashboard = resource.data;
  const update = async (next: MonitoringDashboard) => {
    if (!writable || busy) return;
    setBusy(true); setError('');
    try {
      const saved = await request<MonitoringDashboard>(`/monitoring/dashboards/${id}`, { method: 'PUT', body: JSON.stringify(next) });
      resource.setData(saved); onChange(saved);
    } finally { setBusy(false); }
  };
  const action = (next: MonitoringDashboard) => { void update(next).catch(err => { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }); };
  const remove = async () => {
    if (!dashboard || !writable || busy || !window.confirm(t('monitoring.deleteDashboardConfirm', { name: dashboard.name }))) return;
    setBusy(true); setError('');
    try { await request(`/monitoring/dashboards/${id}`, { method: 'DELETE' }); onDelete(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <>
    <ResourceState loading={resource.loading} error={resource.error} retry={resource.reload} />
    {error && <div className="alert alert-danger" role="alert">{error}<p className="small mb-0 mt-1">{t('monitoring.conflictHint')}</p><button className="btn btn-sm btn-outline-light mt-2" disabled={busy} onClick={() => { setError(''); resource.reload(); }}>{t('common.refresh')}</button></div>}
    {dashboard && <>
      <div className="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3"><h2 className="h5 mb-0 text-break">{dashboard.name}</h2>
        {writable && <div className="d-flex gap-2 flex-wrap">
          <button className="btn btn-sm btn-outline-secondary" disabled={busy} onClick={() => setRenaming(dashboard)}>{t('monitoring.rename')}</button>
          <button className="btn btn-sm btn-outline-danger" disabled={busy} onClick={() => { void remove(); }}>{t('common.delete')}</button>
          <button className="btn btn-sm btn-primary" disabled={busy || dashboard.widgets.length >= MAX_WIDGETS} onClick={() => setEditing({ widget: createWidget(newWidgetID()), dashboard })}>{t('monitoring.addWidget')}</button>
        </div>}
      </div>
      {dashboard.widgets.length >= MAX_WIDGETS && <p className="small muted">{t('monitoring.widgetLimit', { max: MAX_WIDGETS })}</p>}
      {!dashboard.widgets.length && <div className="panel muted py-5 text-center">{t('monitoring.noWidgets')}</div>}
      <div className="row g-3">{dashboard.widgets.map((widget, index) => <div className={widget.width === 12 ? 'col-12' : 'col-12 col-xl-6'} key={widget.id}>
        <section className="panel h-100 mb-0 monitoring-widget" aria-label={widget.title}>
          <div className="d-flex align-items-start justify-content-between gap-2 mb-3"><h3 className="h6 text-break mb-0">{widget.title}</h3>
            {writable && <div className="d-flex flex-wrap justify-content-end gap-1">
              <button className="btn btn-sm btn-outline-secondary" disabled={busy || index === 0} title={t('monitoring.moveUp')} aria-label={`${t('monitoring.moveUp')}: ${widget.title}`} onClick={() => action({ ...dashboard, widgets: moveWidget(dashboard.widgets, index, -1) })}>↑</button>
              <button className="btn btn-sm btn-outline-secondary" disabled={busy || index === dashboard.widgets.length - 1} title={t('monitoring.moveDown')} aria-label={`${t('monitoring.moveDown')}: ${widget.title}`} onClick={() => action({ ...dashboard, widgets: moveWidget(dashboard.widgets, index, 1) })}>↓</button>
              <button className="btn btn-sm btn-outline-secondary" disabled={busy} onClick={() => action({ ...dashboard, widgets: dashboard.widgets.map(w => w.id === widget.id ? { ...w, width: w.width === 6 ? 12 : 6 } : w) })}>{t(widget.width === 6 ? 'monitoring.fullWidth' : 'monitoring.halfWidth')}</button>
              <button className="btn btn-sm btn-outline-secondary" disabled={busy} onClick={() => setEditing({ widget, dashboard })}>{t('common.edit')}</button>
              <button className="btn btn-sm btn-outline-danger" disabled={busy} aria-label={`${t('common.delete')}: ${widget.title}`} onClick={() => { if (window.confirm(t('monitoring.deleteWidgetConfirm', { name: widget.title }))) action({ ...dashboard, widgets: dashboard.widgets.filter(w => w.id !== widget.id) }); }}>{t('common.delete')}</button>
            </div>}
          </div>
          <WidgetQuery key={JSON.stringify(widget)} widget={widget} request={request} refresh={refresh} />
        </section>
      </div>)}</div>
    </>}
    {renaming && writable && <NameEditor initial={renaming.name} title={t('monitoring.rename')} onClose={() => setRenaming(null)} onSave={name => update({ ...renaming, name })} />}
    {editing && writable && <WidgetWizard initial={editing.widget} devices={devices} request={request} onClose={() => setEditing(null)} onSave={widget => {
      const base = editing.dashboard;
      const exists = base.widgets.some(w => w.id === widget.id);
      return update({ ...base, widgets: exists ? base.widgets.map(w => w.id === widget.id ? widget : w) : [...base.widgets, widget] });
    }} />}
  </>;
}
function NameEditor({ initial, title, onSave, onClose }: { initial: string; title: string; onSave: (name: string) => Promise<void>; onClose: () => void }) {
  const t = useT();
  const [name, setName] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const save = async () => {
    if (!name.trim()) { setError(t('monitoring.validation.title')); return; }
    setBusy(true); setError('');
    try { await onSave(name.trim()); onClose(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <Editor title={title} busy={busy} onClose={onClose}><form onSubmit={e => { e.preventDefault(); void save(); }}>
    <Field label={t('common.name')}><input autoFocus className="form-control" value={name} onChange={e => setName(e.target.value)} maxLength={100} required disabled={busy} /></Field>
    {error && <div className="alert alert-danger" role="alert">{error}<p className="small mb-0">{t('monitoring.conflictHint')}</p></div>}
    <FormActions busy={busy} onClose={onClose} />
  </form></Editor>;
}
