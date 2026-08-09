export type Role = 'admin' | 'operator' | 'viewer';

export interface User {
  id: number;
  login: string;
  email: string;
  display_name?: string;
  role: Role;
  disabled?: boolean;
  email_verified?: boolean;
  auth_provider?: string;
}

export interface APIToken {
  id: number;
  user_id: number;
  name: string;
  token?: string;
  token_prefix: string;
  created_at: number;
  last_used_at?: number | null;
}

export interface ProductMethodParam {
  name: string;
  label?: string;
  type?: string;
  default?: unknown;
  required?: boolean;
}

export interface ProductMethod {
  name: string;
  label?: string;
  description?: string;
  params?: ProductMethodParam[];
}

export interface ProductMetric {
  key: string;
  label?: string;
  group?: string;
  format?: string;
  unit?: string;
  description?: string;
  precision?: number;
}

export interface ProductAttribute {
  key: string;
  label?: string;
  scope?: string;
  type?: string;
  unit?: string;
  writable?: boolean;
  description?: string;
}

export interface Product {
  id: number;
  product_id: string;
  name: string;
  description?: string;
  topic_style_default: string;
  auto_ota?: boolean;
  methods?: ProductMethod[];
  metrics?: ProductMetric[];
  attributes?: ProductAttribute[];
}

export interface Device {
  id: number;
  product_id: string;
  device_id: string;
  name: string;
  description?: string;
  status: string;
  token_prefix?: string;
  topic_style: string;
  fw_version?: string;
  auto_ota?: boolean;
  last_seen_at?: number | null;
  enabled?: boolean;
}

export interface DeviceProvision {
  wifi: { ssid: string; password: string };
  crearts: {
    product: string;
    device: string;
    host: string;
    port: number;
    token: string;
    tls: boolean;
    topic_short: boolean;
  };
}

export interface DeviceCredentials {
  product_id: string;
  device_id: string;
  access_token: string;
  username?: string;
  client_id: string;
  broker_host: string;
  broker_port: number;
  topic_style: string;
  snippet?: string;
  provision?: DeviceProvision;
}

export interface DeviceDetail {
  device: Device;
  product?: Product;
  reported?: Record<string, unknown>;
  desired?: Record<string, unknown>;
  latest_telemetry?: Record<string, TelemetrySample>;
  recent_events?: EventRow[];
  connection: {
    client_id: string;
    username?: string;
    broker_host: string;
    broker_port: number;
    topic_style: string;
    token_prefix?: string;
    note?: string;
    mqtt_session?: MQTTClient | null;
  };
}

export interface TelemetrySample {
  key?: string;
  value_num?: number | null;
  value_text?: string | null;
  value_bool?: boolean | null;
  ts: number;
}

export interface EventRow {
  id?: number;
  device_pk?: number;
  ts: number;
  product_id: string;
  device_id: string;
  type?: string;
  severity: string;
  code: string;
  message?: string;
  data_json?: string | null;
}

export interface LogRow {
  ts: number;
  level: string;
  product_id: string;
  device_id: string;
  module: string;
  message: string;
}

export interface PluginFormOption {
  value: string;
  label: string;
}

export interface PluginFormField {
  key: string;
  type: 'number' | 'string' | 'select' | 'checkbox' | 'time' | 'timezone' | 'json' | 'weekdays' | 'object' | 'array' | 'break';
  label?: string;
  help?: string;
  min?: number;
  max?: number;
  options?: PluginFormOption[];
  options_from?: 'effects';
  disabled_when?: Record<string, unknown>;
  fields?: PluginFormField[];
  add?: unknown;
  add_label?: string;
  remove_label?: string;
  placeholder?: string;
  suggest?: number[];
  wide?: boolean;
}

export interface PluginInfo {
  id: string;
  name: string;
  description: string;
  default_config?: Record<string, unknown>;
  effects?: string[];
  form?: PluginFormField[];
}

export interface DevicePluginBinding {
  id: number;
  device_pk: number;
  plugin_id: string;
  enabled: boolean;
  config: Record<string, unknown>;
  name?: string;
  description?: string;
  effects?: string[];
  form?: PluginFormField[];
  running?: boolean;
  last_error?: string;
  updated_at?: number;
}

export interface Firmware {
  id: number;
  product_id: string;
  module: string;
  version: string;
  size: number;
  url: string;
  sha256?: string;
  download_token?: string;
  hosted?: boolean;
  created_at?: number;
}

export interface OTAJob {
  id: number;
  device_pk: number;
  product_id?: string;
  device_id?: string;
  firmware_id: number;
  firmware_version?: string;
  module?: string;
  status: string;
  step: number;
  desc: string;
  created_at: number;
  updated_at: number;
}

export interface OTANotifyResult {
  firmware_id: number;
  started: number;
  skipped: number;
  failed: number;
  jobs: OTAJob[];
  errors?: string[];
}

export interface MQTTClient {
  client_id: string;
  username: string;
  remote: string;
  listener?: string;
  kind: string;
  device_pk?: number;
  protocol?: number;
  clean?: boolean;
  subscriptions?: string[];
  connected_at?: number;
  last_seen_at?: number;
}

export interface MQTTEvent {
  ts: number;
  level: string;
  kind: string;
  client_id?: string;
  username?: string;
  remote?: string;
  topic?: string;
  message?: string;
  bytes?: number;
}

export interface MQTTStatus {
  listening: boolean;
  addr?: string;
  ws_addr?: string;
  client_count: number;
  clients: MQTTClient[];
  events: MQTTEvent[];
}

export interface Overview {
  online_devices: number;
  offline_devices: number;
  unknown_devices?: number;
  total_devices: number;
  recent_events_24h: number;
  mqtt_connected: boolean;
  mqtt_clients?: number;
  platform_name?: string;
  nodered_url?: string;
}

export interface Settings {
  platform_name: string;
  telemetry_retention_days: number;
  rpc_default_timeout_ms: number;
  broker_host: string;
  rules_bus_enabled: boolean;
  nodered_url: string;
  public_base_url?: string;
  timezone?: string;
  register_enabled?: boolean;
  require_email_verify?: boolean;
  google_enabled?: boolean;
}

export interface AuthConfig {
  register_enabled?: boolean;
  google_enabled?: boolean;
  require_email_verify?: boolean;
}
