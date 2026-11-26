import { useState } from 'react'
import { FileUp, ShieldCheck, Upload } from 'lucide-react'
import { formatBytes } from '../../shared/lib/format'
import { topLevelFolders, type UploadItem } from '../../shared/lib/upload-items'
import { Modal } from '../../shared/ui/modal'

export function UploadDialog({
  host,
  items,
  suggestedDirectory,
  fromShell,
  onClose,
  onConfirm,
}: {
  host: string
  items: UploadItem[]
  suggestedDirectory: string
  fromShell: boolean
  onClose: () => void
  onConfirm: (directory: string) => void
}) {
  const [directory, setDirectory] = useState(suggestedDirectory === '.' ? '' : suggestedDirectory)
  const folders = topLevelFolders(items)
  return (
    <Modal
      title="确认上传目标"
      description="上传文件，不会向终端发送命令或执行文件。"
      onClose={onClose}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          if (directory.startsWith('/')) onConfirm(directory)
        }}
      >
        <div className="modal-body form-stack">
          <div className="upload-summary">
            <FileUp size={25} />
            <div>
              <strong>
                {items.length} 个文件{folders.length > 0 && ` · ${folders.length} 个文件夹`}
              </strong>
              <span>
                {formatBytes(items.reduce((total, item) => total + item.file.size, 0))} ·
                不覆盖同名文件
              </span>
            </div>
          </div>
          <div className="upload-file-names">
            {items.slice(0, 200).map((item, index) => (
              <span key={`${item.relativePath}-${index}`}>{item.relativePath}</span>
            ))}
            {items.length > 200 && <span>…另有 {items.length - 200} 个文件</span>}
          </div>
          <label>
            目标主机
            <input readOnly value={host} />
          </label>
          <label>
            远端目录（绝对路径）
            <input
              autoFocus
              required
              pattern="/.*"
              placeholder="例如 /home/ubuntu"
              value={directory}
              onChange={(event) => setDirectory(event.target.value)}
              spellCheck={false}
            />
          </label>
          {folders.length > 0 && (
            <div className="info-box">
              <FileUp size={18} />
              <span>
                将在目标目录下创建 {folders.join('、')}{' '}
                及其子目录；已存在的目录会被沿用，但已存在的同名文件不会被覆盖，相应文件会单独失败。
              </span>
            </div>
          )}
          <div className="info-box">
            <ShieldCheck size={18} />
            <span>
              {fromShell
                ? '默认目录来自 Shell 最近的报告，仍请核对目标。'
                : '未假定为终端当前目录。请核对文件面板目录，或填写正确路径。'}
            </span>
          </div>
        </div>
        <footer className="modal-footer">
          <button type="button" className="button secondary" onClick={onClose}>
            取消
          </button>
          <button className="button primary" disabled={!directory.startsWith('/')}>
            <Upload size={15} />
            确认上传
          </button>
        </footer>
      </form>
    </Modal>
  )
}
