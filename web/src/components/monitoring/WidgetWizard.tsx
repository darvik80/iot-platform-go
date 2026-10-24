import { useEffect, useState } from 'react';
import { AGGREGATIONS, COLORS, MAX_SOURCES, WINDOWS, WIDGET_TYPES, sourceId, validateSources, validateWidget, type Metric, type QueryResult, type Widget, type WidgetType } from '../../api/monitoring';
import type { Device } from '../../types';
import { useLang, useT } from '../../i18n/I18nContext';
import { isCancelled, type MonitoringRequest, useMonitoringResource } from './hooks';
import { Editor, Field, ResourceState } from './Shared';
import { WidgetView } from './WidgetView';

export function WidgetWizard({ initial, devices, request, onSave, onClose }: { initial: Widget; devices: Device[]; request: MonitoringRequest; onSave: (widget: Widget) => Promise<void>; onClose: () => void }) {
  const t = useT();
  const { lang } = useLang();
  const [widget, setWidget] = useState<Widget>(() => ({ ...initial, sources: initial.sources.map(s => ({ ...s })) }));
  const [step, setStep] = useState(0);
  const [deviceId, setDeviceId] = useState(initial.sources[0]?.device_id || devices[0]?.id || 0);
  const [catalog, setCatalog] = useState<Record<number, Metric[]>>({});
  const [loading, setLoading] = useState(true);
  const [catalogError, setCatalogError] = useState('');
  const [retry, setRetry] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const idsKey = JSON.stringify([...new Set([deviceId, ...widget.sources.map(s => s.device_id)])].filter(Boolean).sort((a, b) => a - b));
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setCatalogError('');
    void Promise.all((JSON.parse(idsKey) as number[]).map(async id => [id, await request<Metric[]>(`/monitoring/metrics?device_id=${id}`, { signal: controller.signal }) || []] as const))
      .then(entries => { if (!controller.signal.aborted) setCatalog(Object.fromEntries(entries)); })
      .catch(err => { if (!controller.signal.aborted && !isCancelled(err)) setCatalogError(err instanceof Error ? err.message : String(err)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [idsKey, request, retry]);
  const kinds = Object.fromEntries(Object.entries(catalog).flatMap(([id, metrics]) => metrics.map(metric => [sourceId({ device_id: Number(id), key: metric.key }), metric.kind])));
  const validation = validateWidget(widget, kinds);
  const preview = useMonitoringResource<QueryResult>(request, step === 2 ? '/monitoring/query' : null, 0, false, JSON.stringify({ widget }));
  const change = (patch: Partial<Widget>) => { setWidget(current => ({ ...current, ...patch })); setError(''); };
  const missing = widget.sources.some(source => !kinds[sourceId(source)]);
  const sourceError = validateSources(widget.sources) || (missing ? 'monitoring.validation.metric' : '');
  const next = () => {
    const problem = step === 0 ? sourceError : validation;
    if (problem) { setError(t(problem)); return; }
    setError(''); setStep(current => current + 1);
  };
  const save = async () => {
    if (validation || !preview.data || preview.loading || preview.error) return;
    setBusy(true); setError('');
    try { await onSave({ ...widget, title: widget.title.trim() }); onClose(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <Editor title={t(initial.title ? 'monitoring.editWidget' : 'monitoring.addWidget')} busy={busy} onClose={onClose}>
    <ol className="monitoring-steps mb-4">{['sources', 'configure', 'preview'].map((name, index) => <li key={name} className={step === index ? 'active' : ''} aria-current={step === index ? 'step' : undefined}>{t(`monitoring.step.${name}`)}</li>)}</ol>
    <form onSubmit={e => { e.preventDefault(); if (step < 2) next(); else void save(); }}>
      <fieldset disabled={busy}>
        {step === 0 && <>
          <p className="small muted">{t('monitoring.sourceHelp', { max: MAX_SOURCES })}</p>
          <Field label={t('common.device')}><select className="form-select" value={deviceId} onChange={e => setDeviceId(Number(e.target.value))}>
            <option value={0} disabled>{t('monitoring.chooseDevice')}</option>{devices.map(d => <option key={d.id} value={d.id}>{d.name || d.device_id}</option>)}
          </select></Field>
          <ResourceState loading={loading} error={catalogError} retry={() => setRetry(v => v + 1)} />
          {!loading && !catalogError && !(catalog[deviceId] || []).length && <p className="muted">{t('monitoring.noMetrics')}</p>}
          {!loading && !catalogError && <div className="monitoring-metric-list">{(catalog[deviceId] || []).map(metric => {
            const source = { device_id: deviceId, key: metric.key };
            const checked = widget.sources.some(s => sourceId(s) === sourceId(source));
            return <label key={metric.key} className="d-flex align-items-start gap-2 border-bottom py-2">
              <input type="checkbox" className="form-check-input" checked={checked} disabled={!checked && widget.sources.length >= MAX_SOURCES} onChange={e => change({ sources: e.target.checked ? [...widget.sources, { ...source, label: `${devices.find(d => d.id === deviceId)?.name || deviceId}: ${metric.key}`, color: COLORS[widget.sources.length % COLORS.length] }] : widget.sources.filter(s => sourceId(s) !== sourceId(source)) })} />
              <span><span className="mono">{metric.key}</span> <span className="badge badge-info">{t(`monitoring.metricKind.${metric.kind}`)}</span><small className="d-block muted">{t('monitoring.lastSeen')}: {new Date(metric.last_seen).toLocaleString(lang)}</small></span>
            </label>;
          })}</div>}
          <div className="small muted mt-3">{t('monitoring.selectedSources', { count: widget.sources.length, max: MAX_SOURCES })}</div>
          <ul className="list-unstyled mt-2">{widget.sources.map(source => <li key={sourceId(source)} className="d-flex align-items-center justify-content-between gap-2 mb-1"><span>{source.label || source.key}</span><button type="button" className="btn btn-sm btn-outline-danger" aria-label={`${t('common.delete')}: ${source.label || source.key}`} onClick={() => change({ sources: widget.sources.filter(s => sourceId(s) !== sourceId(source)) })}>{t('common.delete')}</button></li>)}</ul>
        </>}
        {step === 1 && <>
          <Field label={t('monitoring.widgetTitle')}><input className="form-control" autoFocus required maxLength={100} value={widget.title} onChange={e => change({ title: e.target.value })} /></Field>
          <div className="row"><div className="col-sm-6"><Field label={t('monitoring.widgetType')}><select className="form-select" value={widget.type} onChange={e => change({ type: e.target.value as WidgetType })}>{(Object.keys(WIDGET_TYPES) as WidgetType[]).map(type => <option key={type} value={type}>{t(WIDGET_TYPES[type].label)}</option>)}</select></Field></div>
          <div className="col-sm-6"><Field label={t('monitoring.window')}><select className="form-select" value={widget.window_seconds} onChange={e => change({ window_seconds: Number(e.target.value) })}>{!WINDOWS.some(w => w === widget.window_seconds) && <option value={widget.window_seconds}>{t('monitoring.seconds', { count: widget.window_seconds })}</option>}{WINDOWS.map(seconds => <option key={seconds} value={seconds}>{t(`monitoring.window.${seconds}`)}</option>)}</select></Field></div></div>
          <div className="row"><div className="col-sm-6"><Field label={t('monitoring.aggregation')} hint={t('monitoring.numericHelp')}><select className="form-select" value={widget.aggregation} onChange={e => change({ aggregation: e.target.value as Widget['aggregation'] })}>{AGGREGATIONS.map(a => <option key={a} value={a}>{t(`monitoring.aggregation.${a}`)}</option>)}</select></Field></div>
          <div className="col-sm-6"><Field label={t('monitoring.width')}><select className="form-select" value={widget.width} onChange={e => change({ width: Number(e.target.value) as 6 | 12 })}><option value={6}>{t('monitoring.halfWidth')}</option><option value={12}>{t('monitoring.fullWidth')}</option></select></Field></div></div>
          {WIDGET_TYPES[widget.type].view === 'gauge' && <div className="row">{(['min', 'max'] as const).map(key => <div className="col-sm-6" key={key}><Field label={t(`monitoring.${key}`)}><input className="form-control" type="number" step="any" required value={Number.isNaN(widget[key]) ? '' : widget[key]} onChange={e => change({ [key]: e.target.valueAsNumber })} /></Field></div>)}</div>}
          {widget.sources.map((source, i) => <div className="row align-items-start" key={sourceId(source)}><div className="col-9"><Field label={`${t('monitoring.label')}: ${source.key}`}><input className="form-control" value={source.label} maxLength={100} onChange={e => change({ sources: widget.sources.map((s, index) => index === i ? { ...s, label: e.target.value } : s) })} /></Field></div><div className="col-3"><Field label={t('monitoring.color')}><input className="form-control form-control-color" type="color" value={source.color} onChange={e => change({ sources: widget.sources.map((s, index) => index === i ? { ...s, color: e.target.value } : s) })} /></Field></div></div>)}
          {validation && <p className="small text-warning" role="status">{t(validation)}</p>}
        </>}
        {step === 2 && <><h3 className="h6">{widget.title}</h3><p className="small muted">{t('monitoring.previewHelp')}</p><ResourceState loading={preview.loading} error={preview.error} retry={preview.reload} />{preview.data && <WidgetView widget={widget} result={preview.data} />}</>}
      </fieldset>
      {error && <div className="alert alert-danger mt-3" role="alert">{error}</div>}
      <div className="d-flex justify-content-between gap-2 mt-4">
        <button type="button" className="btn btn-outline-secondary" disabled={busy} onClick={onClose}>{t('common.cancel')}</button>
        <div className="d-flex gap-2">{step > 0 && <button type="button" className="btn btn-outline-secondary" disabled={busy} onClick={() => { setError(''); setStep(v => v - 1); }}>{t('common.back')}</button>}
        <button type="submit" className="btn btn-primary" disabled={busy || (step === 0 && (loading || !!catalogError || !!sourceError)) || (step === 1 && !!validation) || (step === 2 && (!preview.data || preview.loading || !!preview.error))}>{busy ? t('common.loading') : t(step === 2 ? 'common.save' : 'monitoring.next')}</button></div>
      </div>
    </form>
  </Editor>;
}
