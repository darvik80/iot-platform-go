import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react';

type ToastType = 'success' | 'error' | 'warning' | 'info';

interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
}

interface ToastApi {
  (type: ToastType): (message: string) => void;
  error: (message: string) => void;
  warning: (message: string) => void;
  info: (message: string) => void;
  success: (message: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used inside ToastProvider');
  return ctx;
}

const AUTO_DISMISS_MS: Record<ToastType, number> = {
  success: 3000,
  info: 4000,
  warning: 5000,
  error: 6000,
};

const ICON: Record<ToastType, string> = {
  success: '\u2713',
  error: '\u2717',
  warning: '\u26A0',
  info: '\u2139',
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(0);

  const push = useCallback((type: ToastType, message: string) => {
    const id = ++nextId.current;
    setItems((prev) => [...prev, { id, message, type }]);
    setTimeout(() => {
      setItems((prev) => prev.filter((t) => t.id !== id));
    }, AUTO_DISMISS_MS[type]);
  }, []);

  const api = useMemo(() => {
    const fn = ((type: ToastType) => (msg: string) => push(type, msg)) as ToastApi;
    fn.error = (msg: string) => push('error', msg);
    fn.warning = (msg: string) => push('warning', msg);
    fn.info = (msg: string) => push('info', msg);
    fn.success = (msg: string) => push('success', msg);
    return fn;
  }, [push]);

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="toast-container" style={{ position: 'fixed', bottom: '1rem', right: '1rem', zIndex: 1100, display: 'flex', flexDirection: 'column-reverse', gap: '0.5rem', maxWidth: 380 }}>
        {items.map((t) => (
          <ToastToast key={t.id} item={t} onDismiss={() => dismiss(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

function ToastToast({ item, onDismiss }: { item: ToastItem; onDismiss: () => void }) {
  const colorClass: Record<ToastType, string> = {
    success: 'border-success',
    error: 'border-danger',
    warning: 'border-warning',
    info: 'border-info',
  };
  const iconColor: Record<ToastType, string> = {
    success: 'var(--ok)',
    error: 'var(--crit)',
    warning: '#ffc107',
    info: 'var(--info, #0dcaf0)',
  };
  return (
    <div
      className={`toast-item bg-dark border ${colorClass[item.type]} rounded p-2 pe-3 d-flex align-items-start gap-2 shadow`}
      style={{ animation: 'toast-slide-in 0.2s ease-out', minWidth: 260 }}
    >
      <span style={{ color: iconColor[item.type], fontSize: '1rem', lineHeight: 1, flexShrink: 0, marginTop: 1 }}>
        {ICON[item.type]}
      </span>
      <span className="small flex-grow-1" style={{ wordBreak: 'break-word' }}>{item.message}</span>
      <button
        type="button"
        className="btn-close btn-close-white flex-shrink-0"
        style={{ fontSize: '0.55rem', marginTop: 2 }}
        onClick={onDismiss}
      />
    </div>
  );
}

/** Helper: extract error message the same way across the app. */
export function errorMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
