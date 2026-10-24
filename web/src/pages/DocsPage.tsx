import { useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useT } from '../i18n/I18nContext';

type Section = {
  id: string;
  title: string;
  icon: string;
  content: React.ReactNode;
};

function SectionAnchor({ id }: { id: string }) {
  return <div id={id} style={{ scrollMarginTop: '1rem' }} />;
}

export function DocsPage() {
  const { activeMembership, settings } = useAuth();
  const t = useT();
  const { hash } = useLocation();
  useEffect(() => {
    if (hash) document.getElementById(hash.slice(1))?.scrollIntoView();
  }, [hash]);
  const accountSlug = activeMembership?.account_slug || '{account_slug}';
  const brokerHost = settings?.broker_host || '{broker_host}';
  const mqttPort = settings?.mqtt_port || 1883;
  const wsUrl = settings?.mqtt_ws_url || '{ws_url}';

  const sections: Section[] = [
    {
      id: 'getting-started',
      title: t('docs.gettingStarted.title'),
      icon: '🚀',
      content: (
        <>
          <p>{t('docs.gettingStarted.intro')}</p>
          <h4>{t('docs.gettingStarted.concepts')}</h4>
          <ul>
            <li>
              <strong>{t('docs.gettingStarted.product')}</strong> — {t('docs.gettingStarted.productDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.device')}</strong> — {t('docs.gettingStarted.deviceDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.events')}</strong> — {t('docs.gettingStarted.eventsDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.telemetry')}</strong> — {t('docs.gettingStarted.telemetryDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.attributes')}</strong> — {t('docs.gettingStarted.attributesDescription')}{' '}
              <em>{t('docs.gettingStarted.reported')}</em> {t('docs.gettingStarted.reportedDescription')}{' '}
              <em>{t('docs.gettingStarted.desired')}</em> {t('docs.gettingStarted.desiredDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.rpc')}</strong> — {t('docs.gettingStarted.rpcDescription')}
            </li>
            <li>
              <strong>{t('docs.gettingStarted.ota')}</strong> — {t('docs.gettingStarted.otaDescription')}
            </li>
          </ul>
          <h4>{t('docs.gettingStarted.quickStart')}</h4>
          <ol>
            <li><Link to="/products">{t('docs.gettingStarted.createProduct')}</Link></li>
            <li><Link to="/devices">{t('docs.gettingStarted.registerDevice')}</Link></li>
            <li>{t('docs.gettingStarted.connectDevice')}</li>
            <li><Link to="/">{t('docs.gettingStarted.overview')}</Link></li>
            <li><Link to="/monitoring">{t('docs.monitoring.quickStart')}</Link></li>
          </ol>
        </>
      ),
    },
    {
      id: 'mqtt-connection',
      title: t('docs.mqttConnection.title'),
      icon: '🔌',
      content: (
        <>
          <p>{t('docs.mqttConnection.intro')}</p>
          <h4>{t('docs.mqttConnection.details')}</h4>
          <table className="table table-sm table-bordered">
            <tbody>
              <tr><td><strong>{t('docs.mqttConnection.broker')}</strong></td><td className="mono">{brokerHost}</td></tr>
              <tr><td><strong>{t('docs.mqttConnection.tcpPort')}</strong></td><td className="mono">{mqttPort}</td></tr>
              <tr><td><strong>{t('docs.mqttConnection.wsUrl')}</strong></td><td className="mono">{wsUrl}</td></tr>
              <tr><td><strong>{t('docs.mqttConnection.clientId')}</strong></td><td className="mono">{'{product_id}.{device_id}'}</td></tr>
              <tr><td><strong>{t('docs.mqttConnection.username')}</strong></td><td className="mono">{'{product_id}.{device_id}'}</td></tr>
              <tr><td><strong>{t('docs.mqttConnection.password')}</strong></td><td>{t('docs.mqttConnection.passwordDescription')}</td></tr>
            </tbody>
          </table>
          <h4>{t('docs.mqttConnection.topicStyles')}</h4>
          <p>{t('docs.mqttConnection.topicStylesIntro')}</p>
          <ul>
            <li><strong>{t('docs.mqttConnection.full')}</strong>: <span className="mono">iot/v1/{'{product}/{device}/up/{tail}'}</span></li>
            <li><strong>{t('docs.mqttConnection.short')}</strong>: <span className="mono">v1/me/{'{tail}'}</span></li>
          </ul>
          <p>{t('docs.mqttConnection.topicStyleConfiguration')}</p>
        </>
      ),
    },
    {
      id: 'topic-reference',
      title: t('docs.topicReference.title'),
      icon: '📋',
      content: (
        <>
          <h4>{t('docs.topicReference.uplink')}</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>{t('docs.topicReference.purpose')}</th><th>{t('docs.topicReference.fullStyle')}</th><th>{t('docs.topicReference.shortStyle')}</th></tr></thead>
            <tbody>
              <tr><td>{t('docs.topicReference.status')}</td><td className="mono small">up/status</td><td className="mono small">v1/me/s</td></tr>
              <tr><td>{t('docs.topicReference.telemetry')}</td><td className="mono small">up/telemetry/data</td><td className="mono small">v1/me/t</td></tr>
              <tr><td>{t('docs.topicReference.event')}</td><td className="mono small">up/events/post</td><td className="mono small">v1/me/e</td></tr>
              <tr><td>{t('docs.topicReference.attributesReport')}</td><td className="mono small">up/attributes/report</td><td className="mono small">v1/me/a</td></tr>
              <tr><td>{t('docs.topicReference.rpcResponse')}</td><td className="mono small">up/rpc/response</td><td className="mono small">v1/me/r/res</td></tr>
              <tr><td>{t('docs.topicReference.otaVersion')}</td><td className="mono small">up/ota/version</td><td className="mono small">v1/me/o/ver</td></tr>
              <tr><td>{t('docs.topicReference.logs')}</td><td className="mono small">up/logs/report</td><td className="mono small">v1/me/l</td></tr>
            </tbody>
          </table>
          <h4>{t('docs.topicReference.downlink')}</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>{t('docs.topicReference.purpose')}</th><th>{t('docs.topicReference.fullStyle')}</th><th>{t('docs.topicReference.shortStyle')}</th></tr></thead>
            <tbody>
              <tr><td>{t('docs.topicReference.rpcRequest')}</td><td className="mono small">down/rpc/request</td><td className="mono small">v1/me/r/req</td></tr>
              <tr><td>{t('docs.topicReference.attributesUpdate')}</td><td className="mono small">down/attributes/update</td><td className="mono small">v1/me/a/upd</td></tr>
              <tr><td>{t('docs.topicReference.otaUpdate')}</td><td className="mono small">down/ota/update</td><td className="mono small">v1/me/o/upd</td></tr>
              <tr><td>{t('docs.topicReference.ntpResponse')}</td><td className="mono small">down/ntp/response</td><td className="mono small">v1/me/n/res</td></tr>
            </tbody>
          </table>
          <h4>{t('docs.topicReference.payloadFormats')}</h4>
          <p><strong>{t('docs.topicReference.telemetry')}</strong> — {t('docs.topicReference.telemetryPayload')}</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "temperature": 23.5,\n  "humidity": 65,\n  "battery": 87\n}'}</pre>
          <p><strong>{t('docs.topicReference.event')}</strong> — {t('docs.topicReference.eventPayload')}</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "severity": "warning",\n  "code": "HIGH_TEMP",\n  "message": "Temperature exceeded threshold"\n}'}</pre>
          <p><strong>{t('docs.topicReference.status')}</strong> — {t('docs.topicReference.statusPayload')}</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "status": "online",\n  "fw_version": "1.2.3"\n}'}</pre>
        </>
      ),
    },
    {
      id: 'products-devices',
      title: t('docs.productsDevices.title'),
      icon: '📦',
      content: (
        <>
          <h4>{t('docs.productsDevices.creatingProduct')}</h4>
          <p>
            <Link to="/products">{t('docs.productsDevices.openProducts')}</Link> → <strong>{t('docs.productsDevices.newProduct')}</strong>. {t('docs.productsDevices.define')}
          </p>
          <ul>
            <li><strong>{t('docs.productsDevices.productId')}</strong> — {t('docs.productsDevices.productIdDescription')} <span className="mono">temp-sensor</span>).</li>
            <li><strong>{t('docs.productsDevices.topicStyle')}</strong> — <span className="mono">full</span> {t('docs.productsDevices.or')} <span className="mono">short</span>.</li>
            <li><strong>{t('docs.productsDevices.metrics')}</strong> — {t('docs.productsDevices.metricsDescription')}</li>
            <li><strong>{t('docs.productsDevices.methods')}</strong> — {t('docs.productsDevices.methodsDescription')} (<span className="mono">reboot</span>, <span className="mono">set_mode</span> {t('docs.productsDevices.etc')}).</li>
            <li><strong>{t('docs.productsDevices.attributes')}</strong> — {t('docs.productsDevices.attributesDescription')}</li>
          </ul>
          <h4>{t('docs.productsDevices.registeringDevice')}</h4>
          <p>
            <Link to="/devices">{t('docs.productsDevices.openDevices')}</Link> → <strong>{t('docs.productsDevices.newDevice')}</strong>. {t('docs.productsDevices.deviceSetup')}{' '}
            <span className="text-danger">{t('docs.productsDevices.saveCredentials')}</span>
          </p>
          <h4>{t('docs.productsDevices.lifecycle')}</h4>
          <ul>
            <li><strong>{t('docs.productsDevices.enableDisable')}</strong> — {t('docs.productsDevices.enableDisableDescription')}</li>
            <li><strong>{t('docs.productsDevices.rotateToken')}</strong> — {t('docs.productsDevices.rotateTokenDescription')}</li>
            <li><strong>{t('docs.productsDevices.otaUpdate')}</strong> — {t('docs.productsDevices.otaUpdateDescription')}</li>
          </ul>
        </>
      ),
    },
    {
      id: 'monitoring',
      title: t('docs.monitoring.title'),
      icon: '📊',
      content: (
        <>
          <p>{t('docs.monitoring.intro')}</p>
          <p><Link to="/monitoring">{t('docs.monitoring.open')}</Link></p>
          <h4>{t('docs.monitoring.dashboards')}</h4>
          <ol>
            <li>{t('docs.monitoring.create')}</li>
            <li>{t('docs.monitoring.sources')}</li>
            <li>{t('docs.monitoring.appearance')}</li>
            <li>{t('docs.monitoring.preview')}</li>
          </ol>
          <p>{t('docs.monitoring.values')}</p>
          <p>{t('docs.monitoring.empty')}</p>
          <p>{t('docs.monitoring.limits')}</p>
          <p>{t('docs.monitoring.conflict')}</p>
          <h4>{t('docs.monitoring.channels')}</h4>
          <ul>
            <li>{t('docs.monitoring.email')}</li>
            <li>{t('docs.monitoring.tls')}</li>
            <li>{t('docs.monitoring.telegram')}</li>
            <li>{t('docs.monitoring.secret')}</li>
          </ul>
          <p className="alert alert-warning">{t('docs.monitoring.test')}</p>
          <h4>{t('docs.monitoring.rules')}</h4>
          <p>{t('docs.monitoring.ruleSetup')}</p>
          <ul>
            <li>{t('docs.monitoring.threshold')}</li>
            <li>{t('docs.monitoring.noData')}</li>
            <li>{t('docs.monitoring.offline')}</li>
          </ul>
          <h4>{t('docs.monitoring.intervals')}</h4>
          <div className="table-responsive">
            <table className="table table-sm table-bordered">
              <thead><tr><th>{t('docs.monitoring.field')}</th><th>{t('docs.monitoring.meaning')}</th></tr></thead>
              <tbody>
                <tr><th scope="row">{t('monitoring.missing_seconds')}</th><td>{t('docs.monitoring.missing')}</td></tr>
                <tr><th scope="row">{t('monitoring.for_seconds')}</th><td>{t('docs.monitoring.hold')}</td></tr>
                <tr><th scope="row">{t('monitoring.cooldown_seconds')}</th><td>{t('docs.monitoring.cooldown')}</td></tr>
              </tbody>
            </table>
          </div>
          <p>{t('docs.monitoring.example')}</p>
          <p>{t('docs.monitoring.freshness')}</p>
          <p>{t('docs.monitoring.state')}</p>
          <p>{t('docs.monitoring.disabled')}</p>
          <h4>{t('docs.monitoring.history')}</h4>
          <p>{t('docs.monitoring.background')}</p>
          <p>{t('docs.monitoring.delivery')}</p>
          <p>{t('docs.monitoring.failures')}</p>
          <h4>{t('docs.monitoring.permissions')}</h4>
          <p>{t('docs.monitoring.roles')}</p>
        </>
      ),
    },
    {
      id: 'events-logs',
      title: t('docs.eventsLogs.title'),
      icon: '⚡',
      content: (
        <>
          <h4>{t('docs.eventsLogs.events')}</h4>
          <p>
            {t('docs.eventsLogs.eventsIntro')} <Link to="/events">{t('docs.eventsLogs.openEvents')}</Link>
          </p>
          <table className="table table-sm table-bordered">
            <thead><tr><th>{t('docs.eventsLogs.severity')}</th><th>{t('docs.eventsLogs.meaning')}</th></tr></thead>
            <tbody>
              <tr><td><span className="badge text-bg-info">info</span></td><td>{t('docs.eventsLogs.info')}</td></tr>
              <tr><td><span className="badge text-bg-warning">warning</span></td><td>{t('docs.eventsLogs.warning')}</td></tr>
              <tr><td><span className="badge text-bg-danger">error</span></td><td>{t('docs.eventsLogs.error')}</td></tr>
              <tr><td><span className="badge text-bg-dark">critical</span></td><td>{t('docs.eventsLogs.critical')}</td></tr>
            </tbody>
          </table>
          <h4>{t('docs.eventsLogs.deviceLogs')}</h4>
          <p>
            {t('docs.eventsLogs.logsIntro')} <Link to="/logs">{t('docs.eventsLogs.openLogs')}</Link>{' '}
            {t('docs.eventsLogs.logLevel')} (<span className="mono">debug, info, warn, error</span>) {t('docs.eventsLogs.logModule')}
          </p>
        </>
      ),
    },
    {
      id: 'rpc',
      title: t('docs.rpc.title'),
      icon: '📡',
      content: (
        <>
          <p>{t('docs.rpc.intro')}</p>
          <h4>{t('docs.rpc.using')}</h4>
          <ol>
            <li>{t('docs.rpc.openDevice')}</li>
            <li>{t('docs.rpc.goToTab')} <strong>{t('docs.rpc.tab')}</strong>.</li>
            <li>{t('docs.rpc.selectMethod')}</li>
            <li>{t('docs.rpc.fillParameters')} <strong>{t('docs.rpc.invoke')}</strong>.</li>
          </ol>
          <h4>{t('docs.rpc.howItWorks')}</h4>
          <p>{t('docs.rpc.flow')}</p>
          <p>{t('docs.rpc.timeout', { timeout: settings?.rpc_default_timeout_ms || 30000 })}</p>
        </>
      ),
    },
    {
      id: 'ota',
      title: t('docs.ota.title'),
      icon: '🔄',
      content: (
        <>
          <p>{t('docs.ota.intro')}</p>
          <h4>{t('docs.ota.workflow')}</h4>
          <ol>
            <li>
              <strong>{t('docs.ota.registerFirmware')}</strong> — <Link to="/ota">{t('docs.ota.openOta')}</Link> → <strong>{t('docs.ota.uploadFirmware')}</strong>.{' '}
              {t('docs.ota.firmwareDetails')}
            </li>
            <li>
              <strong>{t('docs.ota.startUpdate')}</strong> — {t('docs.ota.startUpdateDescription')} <strong>{t('docs.ota.button')}</strong> {t('docs.ota.selectFirmware')}
            </li>
            <li>
              <strong>{t('docs.ota.monitor')}</strong> — {t('docs.ota.monitorDescription')}
            </li>
          </ol>
          <h4>{t('docs.ota.autoOta')}</h4>
          <p>
            {t('docs.ota.enableAutoOta')} <strong>{t('docs.ota.autoOta')}</strong>. {t('docs.ota.autoOtaDescription')}
          </p>
        </>
      ),
    },
    {
      id: 'automation',
      title: t('docs.automation.title'),
      icon: '🤖',
      content: (
        <>
          <p>{t('docs.automation.intro')}</p>
          <h4>{t('docs.automation.rulesBus')}</h4>
          <p>{t('docs.automation.rulesBusDescription')}</p>
          <h4>{t('docs.automation.topicPatterns')}</h4>
          <table className="table table-sm table-bordered">
            <tbody>
              <tr>
                <td><strong>{t('docs.automation.subscribe')}</strong></td>
                <td className="mono small">platform/v1/events/{accountSlug}/{'{product}'}/{'{device}'}/{'{kind}'}</td>
              </tr>
              <tr>
                <td><strong>{t('docs.automation.publish')}</strong></td>
                <td className="mono small">platform/v1/commands/{accountSlug}/{'{product}'}/{'{device}'}/{'{kind}'}</td>
              </tr>
            </tbody>
          </table>
          <h4>{t('docs.automation.eventKinds')}</h4>
          <ul>
            <li><span className="mono">status</span> — {t('docs.automation.status')}</li>
            <li><span className="mono">telemetry</span> — {t('docs.automation.telemetry')}</li>
            <li><span className="mono">event</span> — {t('docs.automation.event')}</li>
            <li><span className="mono">attributes/reported</span> — {t('docs.automation.attributes')}</li>
            <li><span className="mono">rpc/result</span> — {t('docs.automation.rpc')}</li>
            <li><span className="mono">ota</span> — {t('docs.automation.ota')}</li>
            <li><span className="mono">log</span> — {t('docs.automation.log')}</li>
          </ul>
          <h4>{t('docs.automation.externalPlugins')}</h4>
          <p>
            <Link to="/settings">{t('docs.automation.registerPlugins')}</Link>{' '}
            {t('docs.automation.pluginCredentials')}
          </p>
          <h4>{t('docs.automation.internalPlugins')}</h4>
          <p>{t('docs.automation.internalPluginsDescription')}</p>
        </>
      ),
    },
    {
      id: 'users-roles',
      title: t('docs.usersRoles.title'),
      icon: '👥',
      content: (
        <>
          <p>{t('docs.usersRoles.intro')}</p>
          <h4>{t('docs.usersRoles.roles')}</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>{t('docs.usersRoles.role')}</th><th>{t('docs.usersRoles.permissions')}</th></tr></thead>
            <tbody>
              <tr><td><strong>{t('docs.usersRoles.admin')}</strong></td><td>{t('docs.usersRoles.adminPermissions')}</td></tr>
              <tr><td><strong>{t('docs.usersRoles.operator')}</strong></td><td>{t('docs.usersRoles.operatorPermissions')}</td></tr>
              <tr><td><strong>{t('docs.usersRoles.viewer')}</strong></td><td>{t('docs.usersRoles.viewerPermissions')}</td></tr>
            </tbody>
          </table>
          <h4>{t('docs.usersRoles.invitations')}</h4>
          <p>
            {t('docs.usersRoles.invitationsDescription')}{' '}
            <Link to="/users">{t('docs.usersRoles.openUsers')}</Link> → <strong>{t('docs.usersRoles.inviteUser')}</strong>.
          </p>
          <h4>{t('docs.usersRoles.accountSwitching')}</h4>
          <p>{t('docs.usersRoles.accountSwitchingDescription')}</p>
        </>
      ),
    },
    {
      id: 'api-tokens',
      title: t('docs.apiTokens.title'),
      icon: '🔑',
      content: (
        <>
          <p>{t('docs.apiTokens.intro')}</p>
          <h4>{t('docs.apiTokens.creating')}</h4>
          <ol>
            <li><Link to="/settings">{t('docs.apiTokens.openSettings')}</Link></li>
            <li>{t('docs.apiTokens.enterName')} <span className="mono">mcp</span>, <span className="mono">ci-script</span>).</li>
            <li>{t('docs.apiTokens.clickCreate')} <strong>{t('docs.apiTokens.createToken')}</strong>.</li>
            <li>{t('docs.apiTokens.copyToken')}</li>
          </ol>
          <h4>{t('docs.apiTokens.using')}</h4>
          <p>{t('docs.apiTokens.header')} <span className="mono">Authorization</span>:</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">Authorization: Bearer YOUR_TOKEN_HERE</pre>
          <p>{t('docs.apiTokens.permissions')}</p>
        </>
      ),
    },
  ];

  return (
    <div>
      <h1 className="h3 mb-3">{t('docs.title')}</h1>

      <div className="row g-4">
        {/* Sidebar navigation */}
        <div className="col-md-3">
          <div className="panel">
            <h2 className="h6 text-uppercase muted mb-2">{t('docs.contents')}</h2>
            <nav className="nav flex-column gap-1">
              {sections.map((sec) => (
                <a key={sec.id} href={`#${sec.id}`} className="nav-link px-2 py-1 small" onClick={(e) => {
                  e.preventDefault();
                  document.getElementById(sec.id)?.scrollIntoView({ behavior: 'smooth' });
                }}>
                  {sec.icon} {sec.title}
                </a>
              ))}
            </nav>
          </div>
        </div>

        {/* Main content */}
        <div className="col-md-9">
          {sections.map((sec) => (
            <div key={sec.id} className="panel mb-4">
              <SectionAnchor id={sec.id} />
              <h2 className="h5 mb-3">
                {sec.icon} {sec.title}
              </h2>
              {sec.content}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
