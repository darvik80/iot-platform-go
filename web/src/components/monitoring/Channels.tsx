import { useState } from 'react';
import { CHANNEL_KINDS, channelPayload, validateChannel, type Channel, type ChannelInput } from '../../api/monitoring';
import { useT } from '../../i18n/I18nContext';
import { isCancelled, type MonitoringRequest, useMonitoringResource } from './hooks';
import { Editor, Field, FormActions, ResourceState } from './Shared';

export function Channels({ request, writable, refresh }: { request: MonitoringRequest; writable: boolean; refresh: number }) {
  const t = useT();
  const channels = useMonitoringResource<Channel[]>(request, '/monitoring/channels', refresh);
  const [editing, setEditing] = useState<{ channel?: Channel } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const act = async (channel: Channel, test: boolean) => {
    if (!writable || busy || !window.confirm(t(test ? 'monitoring.testWarning' : 'monitoring.deleteChannelConfirm', { name: channel.name, destination: channel.destination }))) return;
    setBusy(true); setError(''); setNotice('');
    try {
      await request(`/monitoring/channels/${channel.id}${test ? '/test' : ''}`, { method: test ? 'POST' : 'DELETE' });
      if (test) setNotice(t('monitoring.testComplete')); else channels.reload();
    } catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <>
    <div className="d-flex justify-content-between align-items-center gap-3 mb-3"><p className="small muted mb-0">{t('monitoring.channelHelp')}</p>{writable && <button className="btn btn-primary text-nowrap" disabled={busy} onClick={() => setEditing({})}>{t('monitoring.createChannel')}</button>}</div>
    <ResourceState loading={channels.loading} error={channels.error} empty={!channels.data?.length} retry={channels.reload} />
    {error && <div className="alert alert-danger" role="alert">{error}</div>}{notice && <div className="alert alert-info" role="status">{notice}</div>}
    {!!channels.data?.length && <div className="panel table-responsive"><table className="table table-sm align-middle mb-0"><thead><tr><th>{t('common.name')}</th><th>{t('monitoring.channelKind')}</th><th>{t('monitoring.destination')}</th><th>{t('common.status')}</th>{writable && <th>{t('monitoring.actions')}</th>}</tr></thead><tbody>{channels.data.map(channel => <tr key={channel.id}>
      <td>{channel.name}</td><td>{t(`monitoring.channelKind.${channel.kind}`)}</td><td className="text-break mono">{channel.destination}{channel.kind === 'telegram' && <small className="d-block muted">{t(channel.has_secret ? 'monitoring.secretStored' : 'monitoring.secretMissing')}</small>}</td>
      <td>{t(channel.enabled ? 'monitoring.enabled' : 'monitoring.disabled')}</td>
      {writable && <td><div className="d-flex flex-wrap gap-2"><button className="btn btn-sm btn-outline-secondary" disabled={busy} onClick={() => setEditing({ channel })}>{t('common.edit')}</button><button className="btn btn-sm btn-outline-warning" disabled={busy || !channel.enabled} onClick={() => { void act(channel, true); }}>{t('monitoring.testNotification')}</button><button className="btn btn-sm btn-outline-danger" disabled={busy} onClick={() => { void act(channel, false); }}>{t('common.delete')}</button></div></td>}
    </tr>)}</tbody></table></div>}
    {editing && writable && <ChannelEditor channel={editing.channel} onClose={() => setEditing(null)} onSave={async input => {
      await request(`/monitoring/channels${editing.channel ? `/${editing.channel.id}` : ''}`, { method: editing.channel ? 'PUT' : 'POST', body: JSON.stringify(channelPayload(input)) }); channels.reload();
    }} />}
  </>;
}
function ChannelEditor({ channel, onClose, onSave }: { channel?: Channel; onClose: () => void; onSave: (input: ChannelInput) => Promise<void> }) {
  const t = useT();
  // Deliberately construct a write model: tokens are write-only and never read back.
  const [input, setInput] = useState<ChannelInput>({ name: channel?.name || '', kind: channel?.kind || 'email', destination: channel?.destination || '', enabled: channel?.enabled ?? true, secret: '' });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const change = (patch: Partial<ChannelInput>) => { setInput(current => ({ ...current, ...patch })); setError(''); };
  const save = async () => {
    const problem = validateChannel(input, channel?.kind === input.kind && channel.has_secret);
    if (problem) { setError(t(problem)); return; }
    setBusy(true); setError('');
    try { await onSave(input); onClose(); }
    catch (err) { if (!isCancelled(err)) setError(err instanceof Error ? err.message : String(err)); }
    finally { setBusy(false); }
  };
  return <Editor title={t(channel ? 'monitoring.editChannel' : 'monitoring.createChannel')} busy={busy} onClose={onClose}>
    <form onSubmit={e => { e.preventDefault(); void save(); }}><fieldset disabled={busy}>
      <Field label={t('common.name')}><input autoFocus className="form-control" required maxLength={120} value={input.name} onChange={e => change({ name: e.target.value })} /></Field>
      <Field label={t('monitoring.channelKind')}><select className="form-select" value={input.kind} onChange={e => change({ kind: e.target.value as ChannelInput['kind'], destination: '', secret: '' })}>{CHANNEL_KINDS.map(kind => <option key={kind} value={kind}>{t(`monitoring.channelKind.${kind}`)}</option>)}</select></Field>
      <Field label={t(input.kind === 'email' ? 'common.email' : 'monitoring.chatId')}><input className="form-control" type={input.kind === 'email' ? 'email' : 'text'} required value={input.destination} onChange={e => change({ destination: e.target.value })} /></Field>
      {input.kind === 'telegram' && <Field label={t('monitoring.botToken')} hint={t(channel?.has_secret && channel.kind === input.kind ? 'monitoring.secretPreserve' : 'monitoring.secretHelp')}><input className="form-control" type="password" autoComplete="new-password" value={input.secret} onChange={e => change({ secret: e.target.value })} /></Field>}
      <label className="d-flex gap-2"><input className="form-check-input" type="checkbox" checked={input.enabled} onChange={e => change({ enabled: e.target.checked })} />{t('monitoring.enabled')}</label>
      <p className="small muted mt-3">{t('monitoring.noAutomaticTest')}</p>
    </fieldset>{error && <div className="alert alert-danger" role="alert">{error}</div>}<FormActions busy={busy} onClose={onClose} /></form>
  </Editor>;
}
