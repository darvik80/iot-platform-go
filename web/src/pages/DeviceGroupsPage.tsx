import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useT } from '../i18n/I18nContext';
import { api } from '../api/client';
import { listDeviceGroups, createDeviceGroup, updateDeviceGroup, deleteDeviceGroup, getDeviceGroupDevices, setDeviceGroupDevices, type DeviceGroup, type DeviceGroupInput } from '../api/device_groups';
import type { Device, Product } from '../types';

export function DeviceGroupsPage() {
  const { activeMembership, token } = useAuth();
  return <AccountDeviceGroups key={`${activeMembership?.account_id}:${token}`} />;
}

function AccountDeviceGroups() {
  const t = useT();
  const { activeMembership } = useAuth();
  const [groups, setGroups] = useState<DeviceGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<DeviceGroup | null>(null);
  const [creating, setCreating] = useState(false);
  const [devices, setDevices] = useState<Device[]>([]);
  const [products, setProducts] = useState<Product[]>([]);

  const loadGroups = async () => {
    if (!activeMembership) return;
    setLoading(true);
    setError(null);
    try {
      const [groupsData, devicesData, productsData] = await Promise.all([
        listDeviceGroups(),
        api<Device[]>('/devices'),
        api<Product[]>('/products'),
      ]);
      setGroups(groupsData);
      setDevices(devicesData);
      setProducts(productsData);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadGroups(); }, [activeMembership]);

  const handleCreate = async (input: DeviceGroupInput) => {
    try {
      await createDeviceGroup(input);
      setCreating(false);
      await loadGroups();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleUpdate = async (id: number, input: DeviceGroupInput) => {
    try {
      await updateDeviceGroup(id, input);
      setEditing(null);
      await loadGroups();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm(t('deviceGroups.confirmDelete'))) return;
    try {
      await deleteDeviceGroup(id);
      await loadGroups();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleEditDevices = async (groupId: number, deviceIds: number[]) => {
    try {
      await setDeviceGroupDevices(groupId, deviceIds);
      await loadGroups();
    } catch (e) {
      setError(String(e));
    }
  };

  if (!activeMembership) {
    return <div className="alert alert-warning">{t('monitoring.noAccount')}</div>;
  }

  return <div>
    <div className="d-flex align-items-start justify-content-between gap-3 mb-3">
      <div>
        <h1 className="h3 mb-1">{t('nav.deviceGroups')}</h1>
        <p className="muted small mb-0">
          <Link to="/docs#device-groups">{t('docs.title')}</Link>
        </p>
      </div>
      <button type="button" className="btn btn-primary" onClick={() => setCreating(true)} disabled={creating}>
        {t('deviceGroups.create')}
      </button>
    </div>

    {error && <div className="alert alert-danger">{error}</div>}

    {loading ? <div className="panel muted">{t('common.loading')}</div> : groups.length === 0 ? <div className="panel muted">{t('deviceGroups.empty')}</div> : <div className="panel">
      <table className="table table-hover mb-0">
        <thead>
          <tr>
            <th>{t('deviceGroups.name')}</th>
            <th>{t('deviceGroups.scopeType')}</th>
            <th>{t('deviceGroups.description')}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {groups.map(g => <tr key={g.id}>
            <td><strong>{g.name}</strong></td>
            <td><span className="badge text-bg-secondary">{t(`deviceGroups.scope.${g.scope_type}`)}</span></td>
            <td className="muted small">{g.description}</td>
            <td className="text-end">
              <button type="button" className="btn btn-sm btn-outline-secondary me-2" onClick={() => setEditing(g)}>
                {t('common.edit')}
              </button>
              <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => handleDelete(g.id)}>
                {t('common.delete')}
              </button>
            </td>
          </tr>)}
        </tbody>
      </table>
    </div>}

    {creating && <GroupEditor
      title={t('deviceGroups.create')}
      devices={devices}
      products={products}
      onSave={handleCreate}
      onCancel={() => setCreating(false)}
    />}

    {editing && <GroupEditor
      title={t('deviceGroups.edit')}
      group={editing}
      devices={devices}
      products={products}
      onSave={(input) => handleUpdate(editing.id, input)}
      onCancel={() => setEditing(null)}
      onEditDevices={handleEditDevices}
    />}
  </div>;
}

interface GroupEditorProps {
  title: string;
  group?: DeviceGroup;
  devices: Device[];
  products: Product[];
  onSave: (input: DeviceGroupInput) => void;
  onCancel: () => void;
  onEditDevices?: (groupId: number, deviceIds: number[]) => void;
}

function GroupEditor({ title, group, devices, products, onSave, onCancel, onEditDevices }: GroupEditorProps) {
  const t = useT();
  const [name, setName] = useState(group?.name || '');
  const [description, setDescription] = useState(group?.description || '');
  const [scopeType, setScopeType] = useState<'manual' | 'product' | 'tag'>(group?.scope_type || 'manual');
  const [scopeFilter, setScopeFilter] = useState(group?.scope_filter || '{}');
  const [selectedDevices, setSelectedDevices] = useState<number[]>([]);
  const [loadingDevices, setLoadingDevices] = useState(false);

  const loadDevices = async () => {
    if (!group || group.scope_type !== 'manual') return;
    setLoadingDevices(true);
    try {
      const ids = await getDeviceGroupDevices(group.id);
      setSelectedDevices(ids);
    } catch (e) {
      console.error(e);
    } finally {
      setLoadingDevices(false);
    }
  };

  useEffect(() => { loadDevices(); }, [group]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({ name, description, scope_type: scopeType, scope_filter: scopeFilter });
  };

  const handleDeviceToggle = (deviceId: number) => {
    setSelectedDevices(prev => prev.includes(deviceId) ? prev.filter(id => id !== deviceId) : [...prev, deviceId]);
  };

  const handleSaveDevices = () => {
    if (group && onEditDevices) {
      onEditDevices(group.id, selectedDevices);
    }
  };

  return <div className="modal d-block" style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}>
    <div className="modal-dialog modal-lg">
      <div className="modal-content">
        <div className="modal-header">
          <h5 className="modal-title">{title}</h5>
          <button type="button" className="btn-close" onClick={onCancel}></button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="modal-body">
            <div className="mb-3">
              <label className="form-label">{t('deviceGroups.name')}</label>
              <input type="text" className="form-control" value={name} onChange={e => setName(e.target.value)} required maxLength={100} />
            </div>
            <div className="mb-3">
              <label className="form-label">{t('deviceGroups.description')}</label>
              <textarea className="form-control" value={description} onChange={e => setDescription(e.target.value)} rows={3} maxLength={500} />
            </div>
            <div className="mb-3">
              <label className="form-label">{t('deviceGroups.scopeType')}</label>
              <select className="form-select" value={scopeType} onChange={e => setScopeType(e.target.value as any)} disabled={!!group}>
                <option value="manual">{t('deviceGroups.scope.manual')}</option>
                <option value="product">{t('deviceGroups.scope.product')}</option>
                <option value="tag">{t('deviceGroups.scope.tag')}</option>
              </select>
            </div>
            {scopeType === 'product' && <div className="mb-3">
              <label className="form-label">{t('deviceGroups.productId')}</label>
              <select className="form-select" value={JSON.parse(scopeFilter).product_id || ''} onChange={e => setScopeFilter(JSON.stringify({ product_id: Number(e.target.value) }))} required>
                <option value="">{t('common.select')}</option>
                {products.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
              </select>
            </div>}
            {scopeType === 'tag' && <div className="mb-3">
              <label className="form-label">{t('deviceGroups.tag')}</label>
              <input type="text" className="form-control" value={JSON.parse(scopeFilter).tag || ''} onChange={e => setScopeFilter(JSON.stringify({ tag: e.target.value }))} required />
            </div>}
            {scopeType === 'manual' && group && <div className="mb-3">
              <label className="form-label">{t('deviceGroups.devices')}</label>
              {loadingDevices ? <div className="muted">{t('common.loading')}</div> : <div style={{ maxHeight: '300px', overflowY: 'auto', border: '1px solid var(--bs-border-color)', borderRadius: '0.375rem', padding: '0.5rem' }}>
                {devices.length === 0 ? <div className="muted small">{t('deviceGroups.noDevices')}</div> : devices.map(d => <div key={d.id} className="form-check">
                  <input type="checkbox" className="form-check-input" id={`device-${d.id}`} checked={selectedDevices.includes(d.id)} onChange={() => handleDeviceToggle(d.id)} />
                  <label className="form-check-label" htmlFor={`device-${d.id}`}>{d.name || d.device_id}</label>
                </div>)}
              </div>}
              {group && <button type="button" className="btn btn-sm btn-outline-primary mt-2" onClick={handleSaveDevices} disabled={loadingDevices}>
                {t('common.save')}
              </button>}
            </div>}
          </div>
          <div className="modal-footer">
            <button type="button" className="btn btn-secondary" onClick={onCancel}>{t('common.cancel')}</button>
            <button type="submit" className="btn btn-primary">{t('common.save')}</button>
          </div>
        </form>
      </div>
    </div>
  </div>;
}
