import { useEffect, useId, useRef, type ReactNode } from 'react';
import { useT } from '../../i18n/I18nContext';

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return <label className="d-block mb-3"><span className="form-label">{label}</span>{children}{hint && <span className="d-block small muted mt-1">{hint}</span>}</label>;
}
export function ResourceState({ loading, error, empty, retry }: { loading: boolean; error: string; empty?: boolean; retry?: () => void }) {
  const t = useT();
  return <>
    {error && <div className="alert alert-danger py-2" role="alert">{error}{retry && <button type="button" className="btn btn-sm btn-outline-light ms-2" onClick={retry}>{t('common.refresh')}</button>}</div>}
    {loading && <p className="small muted" role="status">{t('common.loading')}</p>}
    {!loading && !error && empty && <p className="muted py-3">{t('common.noData')}</p>}
  </>;
}
export function Editor({ title, busy, onClose, children }: { title: string; busy: boolean; onClose: () => void; children: ReactNode }) {
  const t = useT();
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => dialog?.close();
  }, []);
  return <dialog ref={ref} className="monitoring-dialog" aria-labelledby={titleId} onCancel={e => { e.preventDefault(); if (!busy) onClose(); }}>
    <div className="d-flex align-items-center justify-content-between gap-3 border-bottom pb-3 mb-3">
      <h2 id={titleId} className="h5 mb-0">{title}</h2>
      <button type="button" className="btn-close btn-close-white" aria-label={t('common.close')} disabled={busy} onClick={onClose} />
    </div>
    {children}
  </dialog>;
}
export function FormActions({ busy, onClose, saveLabel, saveDisabled = false }: { busy: boolean; onClose: () => void; saveLabel?: string; saveDisabled?: boolean }) {
  const t = useT();
  return <div className="d-flex justify-content-end gap-2 mt-3">
    <button type="button" className="btn btn-outline-secondary" disabled={busy} onClick={onClose}>{t('common.cancel')}</button>
    <button type="submit" className="btn btn-primary" disabled={busy || saveDisabled}>{busy ? t('common.loading') : saveLabel || t('common.save')}</button>
  </div>;
}
