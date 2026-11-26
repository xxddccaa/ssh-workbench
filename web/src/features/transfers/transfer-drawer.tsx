import { Check, CircleAlert, FileUp, LoaderCircle, RotateCcw, X } from 'lucide-react'
import { formatBytes } from '../../shared/lib/format'
import type { Transfer } from './use-transfers'

export function TransferDrawer({
  transfers,
  onCancel,
  onRetry,
  onClear,
  onClose,
}: {
  transfers: Transfer[]
  onCancel: (id: string) => void
  onRetry: (id: string) => void
  onClear: () => void
  onClose: () => void
}) {
  return (
    <section className="transfer-drawer" aria-label="传输任务">
      <header className="panel-heading">
        <div>
          <FileUp size={16} />
          <h2>传输任务</h2>
          <span className="subtle-badge">{transfers.length}</span>
        </div>
        <div>
          <button className="text-button" onClick={onClear}>
            清理已结束
          </button>
          <button className="icon-button" aria-label="收起传输任务" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
      </header>
      <div className="transfer-list">
        {transfers.length === 0 ? (
          <div className="transfer-empty">
            暂无传输任务。将文件拖进终端或文件面板，确认目录后上传。
          </div>
        ) : (
          transfers.map((transfer) => (
            <div className="transfer-row" key={transfer.id}>
              <div className={`transfer-state ${transfer.status}`}>
                {transfer.status === 'done' ? (
                  <Check size={17} />
                ) : transfer.status === 'uploading' ? (
                  <LoaderCircle size={17} className="spin" />
                ) : transfer.status === 'error' ? (
                  <CircleAlert size={17} />
                ) : (
                  <FileUp size={17} />
                )}
              </div>
              <div className="transfer-description">
                <strong>
                  {transfer.name}
                  <span>{formatBytes(transfer.size)}</span>
                </strong>
                <small title={`${transfer.host}:${transfer.directory}`}>
                  {transfer.error ?? `${transfer.host} → ${transfer.directory}`}
                </small>
                {transfer.status === 'uploading' && (
                  <div className="progress-track">
                    <span style={{ width: `${transfer.percent}%` }} />
                  </div>
                )}
              </div>
              <span className="transfer-label">
                {transfer.status === 'uploading'
                  ? transfer.percent === 100
                    ? '等待远端完成'
                    : `${transfer.percent}%`
                  : { queued: '等待上传', done: '已完成', error: '失败', cancelled: '已取消' }[
                      transfer.status
                    ]}
              </span>
              {['queued', 'uploading'].includes(transfer.status) ? (
                <button
                  className="icon-button"
                  aria-label={`取消上传 ${transfer.name}`}
                  onClick={() => onCancel(transfer.id)}
                >
                  <X size={15} />
                </button>
              ) : transfer.status !== 'done' ? (
                <button
                  className="icon-button"
                  aria-label={`重试上传 ${transfer.name}`}
                  title="从头重试，不覆盖同名文件"
                  onClick={() => onRetry(transfer.id)}
                >
                  <RotateCcw size={15} />
                </button>
              ) : (
                <span className="button-spacer" />
              )}
            </div>
          ))
        )}
      </div>
    </section>
  )
}
