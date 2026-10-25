import { api } from './client';
import type { Scope } from './monitoring';

export interface DeviceGroup {
  id: number;
  account_id: number;
  name: string;
  description: string;
  scope_type: 'manual' | 'product' | 'tag';
  scope_filter: string;
  created_at: number;
  updated_at: number;
}

export interface DeviceGroupInput {
  name: string;
  description: string;
  scope_type: 'manual' | 'product' | 'tag';
  scope_filter: string;
}

export async function listDeviceGroups(): Promise<DeviceGroup[]> {
  return api<DeviceGroup[]>('/device-groups');
}

export async function getDeviceGroup(id: number): Promise<DeviceGroup> {
  return api<DeviceGroup>(`/device-groups/${id}`);
}

export async function createDeviceGroup(input: DeviceGroupInput): Promise<DeviceGroup> {
  return api<DeviceGroup>('/device-groups', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function updateDeviceGroup(id: number, input: DeviceGroupInput): Promise<DeviceGroup> {
  return api<DeviceGroup>(`/device-groups/${id}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

export async function deleteDeviceGroup(id: number): Promise<void> {
  await api<void>(`/device-groups/${id}`, { method: 'DELETE' });
}

export async function getDeviceGroupDevices(id: number): Promise<number[]> {
  return api<number[]>(`/device-groups/${id}/devices`);
}

export async function setDeviceGroupDevices(id: number, deviceIds: number[]): Promise<void> {
  await api<void>(`/device-groups/${id}/devices`, {
    method: 'PUT',
    body: JSON.stringify({ device_ids: deviceIds }),
  });
}

export function scopeLabel(scope: Scope | null | undefined): string {
  if (!scope || !scope.type) return 'Single device';
  switch (scope.type) {
    case 'device': return `Device #${scope.device_id}`;
    case 'group': return `Group #${scope.group_id}`;
    case 'product': return `Product #${scope.product_id}`;
    case 'tag': return `Tag: ${scope.tag}`;
    default: return 'Unknown scope';
  }
}
