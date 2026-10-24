import type { AlertEvent, Delivery } from '../../api/monitoring';
import { useLang, useT } from '../../i18n/I18nContext';
import { type MonitoringRequest, useMonitoringResource } from './hooks';
import { ResourceState } from './Shared';

export function History({ request, refresh }: { request: MonitoringRequest; refresh: number }) {
  const t = useT();
  const { lang } = useLang();
  const events = useMonitoringResource<AlertEvent[]>(request, '/monitoring/events?limit=100', refresh, true);
  const deliveries = useMonitoringResource<Delivery[]>(request, '/monitoring/deliveries?limit=100', refresh, true);
  const time = (value: number) => value ? new Date(value).toLocaleString(lang) : t('common.noData');
  const status = (value: string) => { const key = `monitoring.state.${value}`; const translated = t(key); return translated === key ? value : translated; };
  return <>
    <p className="small muted">{t('monitoring.historyHelp')}</p>
    <section className="panel"><h2 className="h5">{t('monitoring.events')}</h2><ResourceState loading={events.loading} error={events.error} empty={!events.data?.length} retry={events.reload} />
      {!!events.data?.length && <div className="table-responsive"><table className="table table-sm mb-0"><thead><tr><th>{t('common.id')}</th><th>{t('common.time')}</th><th>{t('common.name')}</th><th>{t('common.status')}</th><th>{t('common.message')}</th></tr></thead><tbody>{events.data.map(event => <tr key={event.id}><td>{event.id}</td><td>{time(event.created_at)}</td><td>{event.name}</td><td>{status(event.state)}</td><td className="text-break">{event.message}</td></tr>)}</tbody></table></div>}
    </section>
    <section className="panel"><h2 className="h5">{t('monitoring.deliveries')}</h2><ResourceState loading={deliveries.loading} error={deliveries.error} empty={!deliveries.data?.length} retry={deliveries.reload} />
      {!!deliveries.data?.length && <div className="table-responsive"><table className="table table-sm mb-0"><thead><tr><th>{t('common.id')}</th><th>{t('monitoring.eventId')}</th><th>{t('monitoring.channelId')}</th><th>{t('common.status')}</th><th>{t('monitoring.attempts')}</th><th>{t('monitoring.lastError')}</th><th>{t('monitoring.nextAttempt')}</th></tr></thead><tbody>{deliveries.data.map(delivery => <tr key={delivery.id}><td>{delivery.id}</td><td>{delivery.event_id}</td><td>{delivery.channel_id}</td><td>{status(delivery.status)}</td><td>{delivery.attempts}</td><td className="text-break">{delivery.last_error || t('common.none')}</td><td>{time(delivery.next_attempt)}</td></tr>)}</tbody></table></div>}
    </section>
  </>;
}
