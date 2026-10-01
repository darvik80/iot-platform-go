const API = '/api/v1';

type Listener = () => void;
const unauthorizedListeners = new Set<Listener>();

export function onUnauthorized(fn: Listener) {
  unauthorizedListeners.add(fn);
  return () => {
    unauthorizedListeners.delete(fn);
  };
}

function notifyUnauthorized() {
  unauthorizedListeners.forEach((fn) => fn());
}

export function getToken() {
  return localStorage.getItem('token') || '';
}

// Wi-Fi credentials remembered by the device provisioning dialog; wiped on sign-out.
export const WIFI_STORAGE_KEY = 'cogitor.wifi';

export function setToken(token: string) {
  if (token) localStorage.setItem('token', token);
  else localStorage.removeItem('token');
}

export async function api<T = unknown>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers = new Headers(opts.headers || {});
  if (!headers.has('Content-Type') && opts.body && !(opts.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  const token = getToken();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(API + path, { ...opts, headers });
  if (res.status === 401) {
    setToken('');
    notifyUnauthorized();
    throw new Error('unauthorized');
  }
  if (res.status === 204) return null as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((data as { error?: string }).error || res.statusText);
  return data as T;
}
