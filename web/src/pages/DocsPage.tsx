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
      title: 'Getting Started',
      icon: '🚀',
      content: (
        <>
          <p>
            Welcome to the IoT Platform. This guide covers the core concepts and workflows for managing
            devices, products, events, and automation.
          </p>
          <h4>Core Concepts</h4>
          <ul>
            <li>
              <strong>Product</strong> — a device model/type (e.g. "Temperature Sensor v2"). Defines metrics, methods, and attributes shared by all devices of this type.
            </li>
            <li>
              <strong>Device</strong> — a physical instance of a product. Each device has unique credentials and communicates via MQTT.
            </li>
            <li>
              <strong>Events</strong> — structured notifications from devices (errors, status changes, custom events). Each event has a severity level and code.
            </li>
            <li>
              <strong>Telemetry</strong> — periodic measurements sent by devices (temperature, humidity, battery level, etc.).
            </li>
            <li>
              <strong>Attributes</strong> — key-value state on a device. <em>Reported</em> attributes come from the device; <em>desired</em> attributes are set by the platform.
            </li>
            <li>
              <strong>RPC</strong> — remote procedure calls. Send commands to devices and receive responses.
            </li>
            <li>
              <strong>OTA</strong> — over-the-air firmware updates. Register firmware binaries and push them to devices.
            </li>
          </ul>
          <h4>Quick Start</h4>
          <ol>
            <li>Create a <Link to="/products">Product</Link> with your device schema (metrics, methods).</li>
            <li>Register a <Link to="/devices">Device</Link> — you'll receive MQTT credentials.</li>
            <li>Connect your device using the provided credentials.</li>
            <li>Monitor telemetry and events on the <Link to="/">Overview</Link> page.</li>
            <li><Link to="/monitoring">{t('docs.monitoring.quickStart')}</Link></li>
          </ol>
        </>
      ),
    },
    {
      id: 'mqtt-connection',
      title: 'MQTT Connection',
      icon: '🔌',
      content: (
        <>
          <p>
            All devices communicate over MQTT. The platform supports both TCP and WebSocket transports.
          </p>
          <h4>Connection Details</h4>
          <table className="table table-sm table-bordered">
            <tbody>
              <tr><td><strong>Broker</strong></td><td className="mono">{brokerHost}</td></tr>
              <tr><td><strong>TCP Port</strong></td><td className="mono">{mqttPort}</td></tr>
              <tr><td><strong>WebSocket URL</strong></td><td className="mono">{wsUrl}</td></tr>
              <tr><td><strong>Client ID</strong></td><td className="mono">{'{product_id}.{device_id}'}</td></tr>
              <tr><td><strong>Username</strong></td><td className="mono">{'{product_id}.{device_id}'}</td></tr>
              <tr><td><strong>Password</strong></td><td>Device access token (shown at creation or token rotation)</td></tr>
            </tbody>
          </table>
          <h4>Topic Styles</h4>
          <p>The platform supports two topic styles:</p>
          <ul>
            <li><strong>Full</strong>: <span className="mono">iot/v1/{'{product}/{device}/up/{tail}'}</span></li>
            <li><strong>Short</strong>: <span className="mono">v1/me/{'{tail}'}</span></li>
          </ul>
          <p>The topic style is configured per-product and can be overridden per-device.</p>
        </>
      ),
    },
    {
      id: 'topic-reference',
      title: 'Topic Reference',
      icon: '📋',
      content: (
        <>
          <h4>Uplink Topics (Device → Platform)</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>Purpose</th><th>Full Style</th><th>Short Style</th></tr></thead>
            <tbody>
              <tr><td>Status</td><td className="mono small">up/status</td><td className="mono small">v1/me/s</td></tr>
              <tr><td>Telemetry</td><td className="mono small">up/telemetry/data</td><td className="mono small">v1/me/t</td></tr>
              <tr><td>Event</td><td className="mono small">up/events/post</td><td className="mono small">v1/me/e</td></tr>
              <tr><td>Attributes report</td><td className="mono small">up/attributes/report</td><td className="mono small">v1/me/a</td></tr>
              <tr><td>RPC response</td><td className="mono small">up/rpc/response</td><td className="mono small">v1/me/r/res</td></tr>
              <tr><td>OTA version</td><td className="mono small">up/ota/version</td><td className="mono small">v1/me/o/ver</td></tr>
              <tr><td>Logs</td><td className="mono small">up/logs/report</td><td className="mono small">v1/me/l</td></tr>
            </tbody>
          </table>
          <h4>Downlink Topics (Platform → Device)</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>Purpose</th><th>Full Style</th><th>Short Style</th></tr></thead>
            <tbody>
              <tr><td>RPC request</td><td className="mono small">down/rpc/request</td><td className="mono small">v1/me/r/req</td></tr>
              <tr><td>Attributes update</td><td className="mono small">down/attributes/update</td><td className="mono small">v1/me/a/upd</td></tr>
              <tr><td>OTA update</td><td className="mono small">down/ota/update</td><td className="mono small">v1/me/o/upd</td></tr>
              <tr><td>NTP response</td><td className="mono small">down/ntp/response</td><td className="mono small">v1/me/n/res</td></tr>
            </tbody>
          </table>
          <h4>Payload Formats</h4>
          <p><strong>Telemetry</strong> — JSON object with key-value pairs:</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "temperature": 23.5,\n  "humidity": 65,\n  "battery": 87\n}'}</pre>
          <p><strong>Event</strong> — JSON with severity, code, and optional message:</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "severity": "warning",\n  "code": "HIGH_TEMP",\n  "message": "Temperature exceeded threshold"\n}'}</pre>
          <p><strong>Status</strong> — JSON with online/offline status:</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">{'{\n  "status": "online",\n  "fw_version": "1.2.3"\n}'}</pre>
        </>
      ),
    },
    {
      id: 'products-devices',
      title: 'Products & Devices',
      icon: '📦',
      content: (
        <>
          <h4>Creating a Product</h4>
          <p>
            Go to <Link to="/products">Products</Link> → <strong>New Product</strong>. Define:
          </p>
          <ul>
            <li><strong>Product ID</strong> — unique identifier used in MQTT topics (e.g. <span className="mono">temp-sensor</span>).</li>
            <li><strong>Topic Style</strong> — <span className="mono">full</span> or <span className="mono">short</span>.</li>
            <li><strong>Metrics</strong> — telemetry keys the device sends (temperature, humidity, etc.).</li>
            <li><strong>Methods</strong> — RPC methods the device supports (reboot, set_mode, etc.).</li>
            <li><strong>Attributes</strong> — device state keys (reported by device or set by platform).</li>
          </ul>
          <h4>Registering a Device</h4>
          <p>
            Go to <Link to="/devices">Devices</Link> → <strong>New Device</strong>. Select the product and provide a device ID.
            After creation you'll see credentials — <span className="text-danger">save them immediately</span>, the access token is shown only once.
          </p>
          <h4>Device Lifecycle</h4>
          <ul>
            <li><strong>Enable/Disable</strong> — disabled devices are rejected by the broker.</li>
            <li><strong>Token Rotate</strong> — generates a new access token, invalidating the previous one.</li>
            <li><strong>OTA Update</strong> — push firmware to the device from the device detail page.</li>
          </ul>
        </>
      ),
    },
    {
      id: 'monitoring',
      title: t('docs.monitoring.title'),
      icon: '',
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
      title: 'Events & Logs',
      icon: '⚡',
      content: (
        <>
          <h4>Events</h4>
          <p>
            Events are structured notifications from devices. View them on the <Link to="/events">Events</Link> page.
          </p>
          <table className="table table-sm table-bordered">
            <thead><tr><th>Severity</th><th>Meaning</th></tr></thead>
            <tbody>
              <tr><td><span className="badge text-bg-info">info</span></td><td>Informational — normal operation</td></tr>
              <tr><td><span className="badge text-bg-warning">warning</span></td><td>Warning — may need attention</td></tr>
              <tr><td><span className="badge text-bg-danger">error</span></td><td>Error — something failed</td></tr>
              <tr><td><span className="badge text-bg-dark">critical</span></td><td>Critical — immediate action required</td></tr>
            </tbody>
          </table>
          <h4>Device Logs</h4>
          <p>
            Logs are free-form messages from device firmware. View on the <Link to="/logs">Logs</Link> page.
            Each log entry has a level (debug, info, warn, error) and a module tag.
          </p>
        </>
      ),
    },
    {
      id: 'rpc',
      title: 'Remote Procedure Calls',
      icon: '📡',
      content: (
        <>
          <p>
            RPC allows you to send commands to devices and receive responses. Methods are defined per-product.
          </p>
          <h4>Using RPC</h4>
          <ol>
            <li>Open a device's detail page.</li>
            <li>Go to the <strong>RPC</strong> tab.</li>
            <li>Select a method defined in the product schema.</li>
            <li>Fill in parameters and click <strong>Invoke</strong>.</li>
          </ol>
          <h4>How it Works</h4>
          <p>
            The platform publishes an RPC request to the device's downlink topic. The device processes it
            and publishes a response to the uplink topic. The platform correlates the response and returns it.
          </p>
          <p>
            Default timeout is {settings?.rpc_default_timeout_ms || 30000}ms (configurable in platform settings).
          </p>
        </>
      ),
    },
    {
      id: 'ota',
      title: 'OTA Updates',
      icon: '🔄',
      content: (
        <>
          <p>
            Over-the-air updates let you push new firmware to devices remotely.
          </p>
          <h4>Workflow</h4>
          <ol>
            <li>
              <strong>Register Firmware</strong> — go to <Link to="/ota">OTA</Link> → <strong>Upload Firmware</strong>.
              Provide product, module, version, and the binary file.
            </li>
            <li>
              <strong>Start Update</strong> — from a device's detail page, click <strong>OTA</strong> and select the firmware.
            </li>
            <li>
              <strong>Monitor</strong> — the OTA job progress is shown in real-time.
            </li>
          </ol>
          <h4>Auto OTA</h4>
          <p>
            Products can have <strong>Auto OTA</strong> enabled. When new firmware is registered for a module,
            all online devices with that module are automatically notified.
          </p>
        </>
      ),
    },
    {
      id: 'automation',
      title: 'Automation & Plugins',
      icon: '🤖',
      content: (
        <>
          <p>
            The platform supports automation through internal plugins and external integrations.
          </p>
          <h4>Rules Bus</h4>
          <p>
            When enabled, the platform publishes all device events to an MQTT rules bus. External services
            can subscribe and react to events in real-time.
          </p>
          <h4>Topic Patterns</h4>
          <table className="table table-sm table-bordered">
            <tbody>
              <tr>
                <td><strong>Subscribe (events)</strong></td>
                <td className="mono small">platform/v1/events/{accountSlug}/{'{product}'}/{'{device}'}/{'{kind}'}</td>
              </tr>
              <tr>
                <td><strong>Publish (commands)</strong></td>
                <td className="mono small">platform/v1/commands/{accountSlug}/{'{product}'}/{'{device}'}/{'{kind}'}</td>
              </tr>
            </tbody>
          </table>
          <h4>Event Kinds</h4>
          <ul>
            <li><span className="mono">status</span> — device online/offline</li>
            <li><span className="mono">telemetry</span> — telemetry data</li>
            <li><span className="mono">event</span> — device events</li>
            <li><span className="mono">attributes/reported</span> — attribute changes</li>
            <li><span className="mono">rpc/result</span> — RPC call results</li>
            <li><span className="mono">ota</span> — OTA status changes</li>
            <li><span className="mono">log</span> — device log messages</li>
          </ul>
          <h4>External Plugins</h4>
          <p>
            Admins can register external plugins on the <Link to="/settings">Settings</Link> page.
            Each plugin gets its own MQTT credentials and can subscribe to the rules bus.
          </p>
          <h4>Internal Plugins</h4>
          <p>
            Built-in plugins run inside the platform process. They can be bound to individual devices
            from the device detail page. Examples: LED garland controller, scheduling, time windows.
          </p>
        </>
      ),
    },
    {
      id: 'users-roles',
      title: 'Users & Roles',
      icon: '👥',
      content: (
        <>
          <p>
            The platform supports multi-tenant access with role-based permissions.
          </p>
          <h4>Roles</h4>
          <table className="table table-sm table-bordered">
            <thead><tr><th>Role</th><th>Permissions</th></tr></thead>
            <tbody>
              <tr><td><strong>Admin</strong></td><td>Full access — manage users, devices, products, settings, plugins</td></tr>
              <tr><td><strong>Operator</strong></td><td>Read + write — manage devices, send RPC, OTA updates</td></tr>
              <tr><td><strong>Viewer</strong></td><td>Read-only — view devices, telemetry, events</td></tr>
            </tbody>
          </table>
          <h4>Invitations</h4>
          <p>
            Admins can invite users by email. The invitee receives a link to create an account and join the account.
            Go to <Link to="/users">Users</Link> → <strong>Invite User</strong>.
          </p>
          <h4>Account Switching</h4>
          <p>
            Users belonging to multiple accounts can switch between them using the account switcher in the sidebar.
          </p>
        </>
      ),
    },
    {
      id: 'api-tokens',
      title: 'API Tokens',
      icon: '🔑',
      content: (
        <>
          <p>
            Long-lived API tokens are useful for scripts, MCP integrations, and automation.
          </p>
          <h4>Creating a Token</h4>
          <ol>
            <li>Go to <Link to="/settings">Settings</Link>.</li>
            <li>Enter a name (e.g. <span className="mono">mcp</span>, <span className="mono">ci-script</span>).</li>
            <li>Click <strong>Create token</strong>.</li>
            <li>Copy the token immediately — it will not be shown again.</li>
          </ol>
          <h4>Using Tokens</h4>
          <p>Include the token in the <span className="mono">Authorization</span> header:</p>
          <pre className="bg-dark border border-secondary rounded p-2 small mono">Authorization: Bearer YOUR_TOKEN_HERE</pre>
          <p>Tokens have the same permissions as your user account and active account selection.</p>
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
