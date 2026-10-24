import { useState } from 'react';
import { RULE_KINDS, validateRule, type AlertRule, type Channel, type Metric, type RuleInput } from '../../api/monitoring';
import type { Device } from '../../types';
import { useLang, useT } from '../../i18n/I18nContext';
import { isCancelled, type MonitoringRequest, useMonitoringResource } from './hooks';
import { Editor, Field, FormActions, ResourceState } from './Shared';

const newRule = (): RuleInput => ({ name: '', device_id: 0, metric: '', kind: 'threshold', operator: 'gt', threshold: 80, recovery: 75, for_seconds: 0, missing_seconds: 300, cooldown_seconds: 300, enabled: true, channel_ids: [] });
export function Alerts({ request, writable, readChannels, devices, refresh }: { request: MonitoringRequest; writable: boolean; readChannels: boolean; devices: Device[]; refresh: number }) {
  const t = useT();
  const { lang } = useLang();
  const rules = useMonitoringResource<AlertRule[]>(request, '/monitoring/rules', refresh, true);
  const channels = useMonitoringResource<Channel[]>(request, readChannels ? '/monitoring/channels' : null, refresh);
  const [editing, setEditing] = useState<{ id?: number; input: RuleInput } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const remove = async (rule: AlertRule) => {
    if (!writable || busy || !window.confirm(t('monitoring.deleteRuleConfirm', { name: rule.name }))) return;
    setBusy(true); setError('');
    try { await request(`/monitoring/rules/${rule.id}`, { method: 'DELETE' }); rules.reload(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <>
    <div className="d-flex justify-content-between align-items-center gap-3 mb-3"><p className="muted small mb-0">{t('monitoring.alertsHelp')}</p>{writable && <button className="btn btn-primary text-nowrap" disabled={busy} onClick={() => setEditing({ input: newRule() })}>{t('monitoring.createRule')}</button>}</div>
    <ResourceState loading={rules.loading} error={rules.error} empty={!rules.data?.length} retry={rules.reload} />
    {error && <div className="alert alert-danger" role="alert">{error}</div>}
    {!!rules.data?.length && <div className="panel table-responsive"><table className="table table-sm align-middle mb-0"><thead><tr><th>{t('common.name')}</th><th>{t('common.device')}</th><th>{t('monitoring.condition')}</th><th>{t('common.status')}</th><th>{t('monitoring.since')}</th>{writable && <th>{t('monitoring.actions')}</th>}</tr></thead>
      <tbody>{rules.data.map(rule => <tr key={rule.id}>
        <td>{rule.name}<small className="d-block muted">{t(rule.enabled ? 'monitoring.enabled' : 'monitoring.disabled')}</small></td>
        <td>{devices.find(d => d.id === rule.device_id)?.name || rule.device_id}</td>
        <td><span>{t(`monitoring.ruleKind.${rule.kind}`)}</span>{rule.kind !== 'offline' && <span className="d-block mono small">{rule.metric}</span>}{rule.kind === 'threshold' ? <small className="d-block">{t(`monitoring.operator.${rule.operator}`)} {rule.threshold} · {t('monitoring.recovery')}: {rule.recovery}</small> : <small className="d-block">{t('monitoring.seconds', { count: rule.missing_seconds })}</small>}</td>
        <td><span className={`badge ${rule.state === 'firing' ? 'badge-critical' : 'badge-info'}`}>{t(`monitoring.state.${rule.state}`) === `monitoring.state.${rule.state}` ? rule.state : t(`monitoring.state.${rule.state}`)}</span></td>
        <td className="small">{rule.since ? new Date(rule.since).toLocaleString(lang) : t('common.noData')}</td>
        {writable && <td><div className="d-flex gap-2"><button className="btn btn-sm btn-outline-secondary" disabled={busy} onClick={() => setEditing({ id: rule.id, input: { name: rule.name, device_id: rule.device_id, metric: rule.metric, kind: rule.kind, operator: rule.operator, threshold: rule.threshold, recovery: rule.recovery, for_seconds: rule.for_seconds, missing_seconds: rule.missing_seconds, cooldown_seconds: rule.cooldown_seconds, enabled: rule.enabled, channel_ids: rule.channel_ids || [] } })}>{t('common.edit')}</button><button className="btn btn-sm btn-outline-danger" disabled={busy} onClick={() => { void remove(rule); }}>{t('common.delete')}</button></div></td>}
      </tr>)}</tbody></table></div>}
    {editing && writable && <RuleEditor initial={editing.input} isNew={!editing.id} devices={devices} channels={channels.data || []} channelsLoading={channels.loading} channelsError={channels.error} reloadChannels={channels.reload} request={request} onClose={() => setEditing(null)} onSave={async input => {
      await request(`/monitoring/rules${editing.id ? `/${editing.id}` : ''}`, { method: editing.id ? 'PUT' : 'POST', body: JSON.stringify(input) }); rules.reload();
    }} />}
  </>;
}
function RuleEditor({ initial, isNew, devices, channels, channelsLoading, channelsError, reloadChannels, request, onSave, onClose }: { initial: RuleInput; isNew: boolean; devices: Device[]; channels: Channel[]; channelsLoading: boolean; channelsError: string; reloadChannels: () => void; request: MonitoringRequest; onSave: (rule: RuleInput) => Promise<void>; onClose: () => void }) {
  const t = useT();
  const [rule, setRule] = useState<RuleInput>(() => ({ ...initial, operator: initial.operator || 'gt' }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const metrics = useMonitoringResource<Metric[]>(request, rule.device_id && rule.kind !== 'offline' ? `/monitoring/metrics?device_id=${rule.device_id}` : null);
  const change = (patch: Partial<RuleInput>) => { setRule(current => ({ ...current, ...patch })); setError(''); };
  const save = async () => {
    const problem = validateRule(rule, metrics.data?.find(m => m.key === rule.metric)?.kind);
    if (problem) { setError(t(problem)); return; }
    if (rule.kind !== 'offline' && !metrics.data?.some(m => m.key === rule.metric)) { setError(t('monitoring.validation.metric')); return; }
    setBusy(true); setError('');
    try { await onSave({ ...rule, name: rule.name.trim(), metric: rule.kind === 'offline' ? '' : rule.metric }); onClose(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <Editor title={t(isNew ? 'monitoring.createRule' : 'monitoring.editRule')} busy={busy} onClose={onClose}>
    <form onSubmit={e => { e.preventDefault(); void save(); }}><fieldset disabled={busy}>
      <Field label={t('common.name')}><input className="form-control" autoFocus required maxLength={120} value={rule.name} onChange={e => change({ name: e.target.value })} /></Field>
      <div className="row"><div className="col-sm-6"><Field label={t('common.device')}><select className="form-select" required value={rule.device_id || ''} onChange={e => change({ device_id: Number(e.target.value), metric: '' })}><option value="" disabled>{t('monitoring.chooseDevice')}</option>{devices.map(d => <option key={d.id} value={d.id}>{d.name || d.device_id}</option>)}</select></Field></div>
      <div className="col-sm-6"><Field label={t('monitoring.condition')}><select className="form-select" value={rule.kind} onChange={e => change({ kind: e.target.value as RuleInput['kind'] })}>{RULE_KINDS.map(kind => <option key={kind} value={kind}>{t(`monitoring.ruleKind.${kind}`)}</option>)}</select></Field></div></div>
      {rule.kind !== 'offline' && <><ResourceState loading={metrics.loading} error={metrics.error} retry={metrics.reload} /><Field label={t('monitoring.metric')} hint={t('monitoring.receivedOnly')}><select className="form-select" required value={rule.metric} onChange={e => change({ metric: e.target.value })}><option value="" disabled>{t('monitoring.chooseMetric')}</option>{(metrics.data || []).map(m => <option key={m.key} value={m.key} disabled={rule.kind === 'threshold' && m.kind !== 'number'}>{m.key} · {t(`monitoring.metricKind.${m.kind}`)}</option>)}</select></Field></>}
      {rule.kind === 'threshold' && <>
        <Field label={t('monitoring.operator')}><select className="form-select" value={rule.operator} onChange={e => change({ operator: e.target.value as RuleInput['operator'] })}><option value="gt">{t('monitoring.operator.gt')}</option><option value="lt">{t('monitoring.operator.lt')}</option></select></Field>
        <div className="row">{(['threshold', 'recovery'] as const).map(key => <div className="col-sm-6" key={key}><Field label={t(`monitoring.${key}`)} hint={key === 'recovery' ? t('monitoring.recoveryHelp') : undefined}><input className="form-control" required type="number" step="any" value={Number.isNaN(rule[key]) ? '' : rule[key]} onChange={e => change({ [key]: e.target.valueAsNumber })} /></Field></div>)}</div>
      </>}
      <div className="row">{(rule.kind === 'threshold' ? ['for_seconds', 'cooldown_seconds'] as const : ['missing_seconds', 'for_seconds', 'cooldown_seconds'] as const).map(key => <div className="col-sm-4" key={key}><Field label={t(`monitoring.${key}`)}><input className="form-control" required type="number" step={1} min={key === 'missing_seconds' ? 1 : key === 'cooldown_seconds' ? 60 : 0} max={604800} value={Number.isNaN(rule[key]) ? '' : rule[key]} onChange={e => change({ [key]: e.target.valueAsNumber })} /></Field></div>)}</div>
      <label className="d-flex gap-2 mb-3"><input className="form-check-input" type="checkbox" checked={rule.enabled} onChange={e => change({ enabled: e.target.checked })} />{t('monitoring.enabled')}</label>
      <fieldset className="border rounded p-3"><legend className="float-none w-auto fs-6 px-1">{t('monitoring.channels')}</legend><ResourceState loading={channelsLoading} error={channelsError} retry={reloadChannels} />
        {!channelsLoading && !channelsError && !channels.length && <p className="small muted">{t('monitoring.noChannelsHint')}</p>}
        {channels.map(channel => <label key={channel.id} className="d-flex gap-2 mb-2"><input className="form-check-input" type="checkbox" checked={rule.channel_ids.includes(channel.id)} onChange={e => change({ channel_ids: e.target.checked ? [...rule.channel_ids, channel.id] : rule.channel_ids.filter(id => id !== channel.id) })} />{channel.name}{!channel.enabled && <span className="muted">({t('monitoring.disabled')})</span>}</label>)}
        <p className="small muted mb-0">{t('monitoring.channelsHelp')}</p>
      </fieldset>
    </fieldset>{error && <div className="alert alert-danger mt-3" role="alert">{error}</div>}<FormActions busy={busy} saveDisabled={channelsLoading || !!channelsError || (rule.kind !== 'offline' && (metrics.loading || !!metrics.error))} onClose={onClose} /></form>
  </Editor>;
}
