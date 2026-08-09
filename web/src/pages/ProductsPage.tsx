import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import type { Product } from '../types';

export function ProductsPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [productId, setProductId] = useState('');
  const [name, setName] = useState('');
  const navigate = useNavigate();

  const load = async () => setProducts(await api<Product[]>('/products'));

  useEffect(() => {
    void load().catch((e) => alert(e.message));
  }, []);

  return (
    <div>
      <h1 className="h3 mb-2">Products</h1>
      <p className="muted">Create a product first, then register devices. Open a product to define RPC methods, metrics and attributes.</p>
      <div className="row g-2 mb-3">
        <div className="col-md-4">
          <input className="form-control" placeholder="product_id (esp32-cam)" value={productId} onChange={(e) => setProductId(e.target.value)} />
        </div>
        <div className="col-md-4">
          <input className="form-control" placeholder="Display name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="col-md-2">
          <button
            type="button"
            className="btn btn-primary w-100"
            onClick={async () => {
              try {
                await api('/products', { method: 'POST', body: JSON.stringify({ product_id: productId, name }) });
                setProductId('');
                setName('');
                await load();
              } catch (e) {
                alert(e instanceof Error ? e.message : String(e));
              }
            }}
          >
            Create
          </button>
        </div>
      </div>
      <div className="panel">
        <div className="table-responsive">
          <table className="table table-hover align-middle">
            <thead>
              <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Methods</th>
                <th>Metrics</th>
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
                      onClick={async (e) => {
                        e.preventDefault();
                        if (!confirm(`Delete product #${p.id}?`)) return;
                        try {
                          await api(`/products/${p.id}`, { method: 'DELETE' });
                          await load();
                        } catch (err) {
                          alert(err instanceof Error ? err.message : String(err));
                        }
                      }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
              {!products.length && (
                <tr>
                  <td colSpan={5} className="muted">
                    No products
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
      <button type="button" className="d-none" onClick={() => navigate('/')} />
    </div>
  );
}
