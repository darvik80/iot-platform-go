import { useState } from 'react';
import { Link } from 'react-router-dom';
import { monitoringPermissions } from '../api/monitoring';
import { useAuth } from '../auth/AuthContext';
import { Alerts } from '../components/monitoring/Alerts';
import { Channels } from '../components/monitoring/Channels';
import { Dashboards } from '../components/monitoring/Dashboards';
import { History } from '../components/monitoring/History';
import { useMonitoringRequest, useMonitoringResource } from '../components/monitoring/hooks';
import { ResourceState } from '../components/monitoring/Shared';
import { useT } from '../i18n/I18nContext';
import type { Device } from '../types';

const TABS = ['dashboards', 'alerts', 'channels', 'history'] as const;
type Tab = typeof TABS[number];
export function MonitoringPage() {
  const { activeAccountId, token } = useAuth();
  return <AccountMonitoring key={`${activeAccountId}:${token}`} />;
}
function AccountMonitoring() {
  const t = useT();
  const { activeMembership } = useAuth();
  const permissions = monitoringPermissions(activeMembership);
  const request = useMonitoringRequest();
  const [tab, setTab] = useState<Tab>('dashboards');
  const [refresh, setRefresh] = useState(0);
  const devices = useMonitoringResource<Device[]>(request, activeMembership ? '/devices' : null, refresh);
  return <div className="monitoring-page">
    <div className="d-flex align-items-start justify-content-between gap-3 mb-3">
      <div><h1 className="h3 mb-1">{t('nav.monitoring')}</h1><p className="muted small mb-0">{t('monitoring.accountScope', { account: activeMembership?.account_name || t('common.none') })} · <Link to="/docs#monitoring">{t('docs.title')}</Link></p></div>
      <button type="button" className="btn btn-outline-secondary" onClick={() => setRefresh(v => v + 1)}>{t('common.refresh')}</button>
    </div>
    {!activeMembership ? <div className="alert alert-warning">{t('monitoring.noAccount')}</div> : <>
      {!permissions.writeDashboards && <p className="small muted">{t('monitoring.readOnly')}</p>}
      <div className="nav nav-tabs mb-3" role="tablist" aria-label={t('nav.monitoring')}>
        {TABS.map((name, index) => <button key={name} type="button" role="tab" id={`monitoring-tab-${name}`} aria-controls="monitoring-tabpanel" aria-selected={tab === name} tabIndex={tab === name ? 0 : -1} className={`nav-link${tab === name ? ' active' : ''}`} onClick={() => setTab(name)} onKeyDown={event => {
          let next = index;
          if (event.key === 'ArrowRight') next = (index + 1) % TABS.length;
          else if (event.key === 'ArrowLeft') next = (index + TABS.length - 1) % TABS.length;
          else if (event.key === 'Home') next = 0;
          else if (event.key === 'End') next = TABS.length - 1;
          else return;
          event.preventDefault(); setTab(TABS[next]); document.getElementById(`monitoring-tab-${TABS[next]}`)?.focus();
        }}>{t(`monitoring.${name}`)}</button>)}
      </div>
      <div id="monitoring-tabpanel" role="tabpanel" aria-labelledby={`monitoring-tab-${tab}`}>
        {(tab === 'dashboards' || tab === 'alerts') && <ResourceState loading={devices.loading} error={devices.error} retry={devices.reload} />}
        {tab === 'dashboards' && <Dashboards request={request} writable={permissions.writeDashboards} devices={devices.data || []} refresh={refresh} />}
        {tab === 'alerts' && <Alerts request={request} writable={permissions.writeRules} readChannels={permissions.readChannels} devices={devices.data || []} refresh={refresh} />}
        {tab === 'channels' && (permissions.readChannels ? <Channels request={request} writable={permissions.writeRules} refresh={refresh} /> : <p className="panel muted">{t('monitoring.adminOnly')}</p>)}
        {tab === 'history' && <History request={request} refresh={refresh} />}
      </div>
      <p className="small muted mt-4">{t('monitoring.autoRefresh')}</p>
    </>}
  </div>;
}
