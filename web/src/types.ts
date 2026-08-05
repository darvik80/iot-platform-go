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
  last_seen_at?: number | null;
  enabled?: boolean;
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
  ts: number;
  product_id: string;
  device_id: string;
  severity: string;
  code: string;
  message?: string;
}

export interface LogRow {
  ts: number;
  level: string;
  product_id: string;
  device_id: string;
  module: string;
  message: string;
}

export interface Firmware {
  product_id: string;
  module: string;
  version: string;
  size: number;
  url: string;
  sha256?: string;
}

export interface Overview {
  online_devices: number;
  offline_devices: number;
  unknown_devices?: number;
  total_devices: number;
  recent_events_24h: number;
  mqtt_connected: boolean;
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
  register_enabled?: boolean;
  require_email_verify?: boolean;
  google_enabled?: boolean;
}

export interface AuthConfig {
  register_enabled?: boolean;
  google_enabled?: boolean;
  require_email_verify?: boolean;
}
