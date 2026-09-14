import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/client';
import { useT } from '../i18n/I18nContext';
import type { Product } from '../types';

export function ProductsPage() {
  const t = useT();
  const [products, setProducts] = useState<Product[]>([]);
  const [productId, setProductId] = useState('');
  const [name, setName] = useState('');
  const [error, setError] = useState('');

  const load = async () => {
    try {
      setProducts(await api<Product[]>('/products'));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    void load().catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  return (
    <div>
      <h1 className="h3 mb-2">{t('products.title')}</h1>
      <p className="muted mb-3">{t('products.description')}</p>
      {error && <div className="alert alert-danger py-2">{error}</div>}
      <div className="row g-2 mb-3">
        <div className="col-md-4">
          <input className="form-control" placeholder="product_id (esp32-cam)" value={productId} onChange={(e) => setProductId(e.target.value)} title="Unique identifier for the product (e.g., esp32-cam)" />
        </div>
        <div className="col-md-4">
          <input className="form-control" placeholder={t('common.displayName')} value={name} onChange={(e) => setName(e.target.value)} title="Human-readable name for the product" />
        </div>
        <div className="col-md-2">
          <button
            type="button"
            className="btn btn-primary w-100"
            title="Create a new product"
            onClick={async () => {
              try {
                await api('/products', { method: 'POST', body: JSON.stringify({ product_id: productId, name }) });
                setProductId('');
                setName('');
                await load();
              } catch (e) {
                setError(e instanceof Error ? e.message : String(e));
              }
            }}
          >
            {t('common.create')}
          </button>
        </div>
      </div>
      <div className="panel">
        <div className="table-responsive">
          <table className="table table-hover align-middle">
            <thead>
              <tr>
                <th>{t('common.id')}</th>
                <th>{t('common.name')}</th>
                <th>{t('products.methods')}</th>
                <th>{t('products.metrics')}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {products.map((p) => (
                <tr key={p.id}>
                  <td className="mono">
                    <Link to={`/products/${p.id}`}>{p.product_id}</Link>
                  </td>
                  <td>{p.name}</td>
                  <td>{(p.methods || []).length}</td>
                  <td>{(p.metrics || []).length}</td>
                  <td>
                    <button
                      type="button"
                      className="btn btn-danger btn-sm"
                      title={t('products.deleteTitle')}
                      onClick={async (e) => {
                        e.preventDefault();
                        if (!confirm(t('products.deleteConfirm', { id: p.id }))) return;
                        try {
                          await api(`/products/${p.id}`, { method: 'DELETE' });
                          await load();
                        } catch (err) {
                          setError(err instanceof Error ? err.message : String(err));
                        }
                      }}
                    >
                      {t('common.delete')}
                    </button>
                  </td>
                </tr>
              ))}
              {!products.length && (
                <tr>
                  <td colSpan={5} className="muted">
                    {t('products.noProducts')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
