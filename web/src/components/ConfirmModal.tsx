import { useT } from '../i18n/I18nContext';

interface Props {
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({ message, confirmLabel, cancelLabel, danger, onConfirm, onCancel }: Props) {
  const t = useT();
  return (
    <>
      <div className="modal-backdrop fade show" />
      <div className="modal fade show d-block" tabIndex={-1} role="dialog">
        <div className="modal-dialog modal-dialog-centered modal-sm">
          <div className="modal-content">
            <div className="modal-header">
              <h5 className="modal-title">{t('common.confirm')}</h5>
              <button type="button" className="btn-close btn-close-white" onClick={onCancel} />
            </div>
            <div className="modal-body">
              <p className="mb-0">{message}</p>
            </div>
            <div className="modal-footer">
              <button type="button" className="btn btn-outline-secondary btn-sm" onClick={onCancel}>
                {cancelLabel || t('common.cancel')}
              </button>
              <button
                type="button"
                className={`btn btn-sm ${danger ? 'btn-danger' : 'btn-primary'}`}
                onClick={onConfirm}
              >
                {confirmLabel || t('common.confirm')}
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
