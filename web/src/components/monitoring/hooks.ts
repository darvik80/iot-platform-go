import { useCallback, useEffect, useLayoutEffect, useRef, useState, type SetStateAction } from 'react';
import { api, getToken } from '../../api/client';
import { accountRequest } from '../../api/monitoring';

export type MonitoringRequest = <T>(path: string, options?: RequestInit) => Promise<T>;
export function isCancelled(error: unknown) { return error instanceof Error && error.name === 'AbortError'; }
export function useMonitoringRequest(): MonitoringRequest {
  const [token] = useState(getToken);
  const scope = useRef(new AbortController());
  useLayoutEffect(() => {
    // Initialize before child passive effects dispatch requests, including StrictMode
    // replay. A parent passive effect would run too late for those child effects.
    scope.current = new AbortController();
    const controller = scope.current;
    return () => controller.abort();
  }, []);
  return useCallback(<T,>(path: string, options: RequestInit = {}) => {
    const signal = options.signal ? AbortSignal.any([scope.current.signal, options.signal]) : scope.current.signal;
    return accountRequest(() => api<T>(path, { ...options, signal }), token, getToken, signal);
  }, [token]);
}

export function useMonitoringResource<T>(request: MonitoringRequest, path: string | null, refresh = 0, poll = false, body?: string) {
  const [data, storeData] = useState<T | null>(null);
  const activeRead = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(!!path);
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  const identity = `${path}\u0000${body || ''}`;
  const [dataIdentity, setDataIdentity] = useState(identity);
  const reload = useCallback(() => setRetry(value => value + 1), []);
  const setData = useCallback((value: SetStateAction<T | null>) => {
    activeRead.current?.abort();
    storeData(value);
    setDataIdentity(identity);
    reload();
  }, [identity, reload]);
  useEffect(() => {
    setError('');
    if (!path) { setLoading(false); return; }
    const controller = new AbortController();
    activeRead.current = controller;
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      setLoading(true);
      try {
        const value = await request<T>(path, { signal: controller.signal, ...(body ? { method: 'POST', body } : {}) });
        if (!controller.signal.aborted) { storeData(value); setDataIdentity(identity); setError(''); }
      } catch (err) {
        if (!controller.signal.aborted && !isCancelled(err)) setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          // Schedule after completion: slow queries cannot overlap or arrive out of order.
          if (poll) timer = setTimeout(() => { void load(); }, 15_000);
        }
      }
    };
    void load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [request, path, refresh, poll, body, retry, identity]);
  return { data: dataIdentity === identity ? data : null, setData, loading, error, reload };
}
