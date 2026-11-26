import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  ClipboardCopy,
  Download,
  FileArchive,
  FileCode2,
  FileText,
  Folder,
  FolderPlus,
  FolderUp,
  KeyRound,
  Link2,
  LoaderCircle,
  Pencil,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { FileEntry, TreeUsage } from '../../shared/api/types'
import { formatBytes, parentPath, shortDate } from '../../shared/lib/format'
import { fromDataTransfer, fromFileList, type UploadItem } from '../../shared/lib/upload-items'
import { Modal } from '../../shared/ui/modal'
import { useNotice } from '../../shared/ui/notices'

type SortKey = 'name' | 'size' | 'modified'
type Operation =
  | { kind: 'mkdir' }
  | { kind: 'rename'; entry: FileEntry }
  | { kind: 'delete'; entry: FileEntry; usage?: TreeUsage }
  | { kind: 'chmod'; entry: FileEntry }

const permissionBits = [
  ['所有者', 0o400, 0o200, 0o100],
  ['组', 0o040, 0o020, 0o010],
  ['其他', 0o004, 0o002, 0o001],
] as const

export function sortEntries(entries: FileEntry[], key: SortKey, ascending: boolean) {
  const direction = ascending ? 1 : -1
  return [...entries].sort((first, second) => {
    if (first.directory !== second.directory) return first.directory ? -1 : 1
    const order =
      key === 'size'
        ? first.size - second.size
        : key === 'modified'
          ? first.modifiedAt.localeCompare(second.modifiedAt)
          : first.name.localeCompare(second.name, 'zh-CN', { numeric: true })
    return order * direction || first.name.localeCompare(second.name, 'zh-CN')
  })
}

export function FilePanel({
  terminalID,
  hostLabel,
  initialPath,
  cwd,
  onDirectory,
  onUpload,
  onEdit,
  onHide,
}: {
  terminalID: string
  hostLabel: string
  initialPath: string
  cwd?: string
  onDirectory: (path: string) => void
  onUpload: (items: UploadItem[], directory: string) => void
  onEdit: (path: string) => void
  onHide: () => void
}) {
  const [directory, setDirectory] = useState(initialPath || '.')
  const [address, setAddress] = useState(initialPath || '.')
  const [search, setSearch] = useState('')
  const [hidden, setHidden] = useState(true)
  const [follow, setFollow] = useState(false)
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [ascending, setAscending] = useState(true)
  const [operation, setOperation] = useState<Operation | null>(null)
  const [name, setName] = useState('')
  const [mode, setMode] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const picker = useRef<HTMLInputElement>(null)
  const folderPicker = useRef<HTMLInputElement>(null)
  const callback = useRef(onDirectory)
  callback.current = onDirectory
  const notify = useNotice()
  const queries = useQueryClient()
  const listing = useQuery({
    queryKey: ['files', terminalID, directory],
    queryFn: ({ signal }) => api.files(terminalID, directory, signal),
    retry: false,
    staleTime: 15000,
  })
  useEffect(() => {
    if (listing.data) {
      setAddress(listing.data.path)
      callback.current(listing.data.path)
    }
  }, [listing.data])
  useEffect(() => {
    if (follow && cwd) setDirectory(cwd)
  }, [follow, cwd])
  useEffect(() => {
    folderPicker.current?.setAttribute('webkitdirectory', '')
  }, [])
  function navigate(path: string) {
    setDirectory(path)
    setFollow(false)
  }
  async function openOperation(next: Operation) {
    setError('')
    setName(next.kind === 'rename' ? next.entry.name : '')
    if (next.kind === 'chmod') setMode(parseInt(next.entry.mode, 8) & 0o777)
    if (next.kind === 'delete' && next.entry.directory) {
      setOperation(next)
      setBusy(true)
      try {
        const usage = await api.usage(terminalID, next.entry.path)
        setOperation({ ...next, usage })
      } catch (problem) {
        setError(messageOf(problem))
      } finally {
        setBusy(false)
      }
      return
    }
    setOperation(next)
  }
  async function mutate() {
    if (!operation || !listing.data) return
    setBusy(true)
    setError('')
    try {
      if (operation.kind === 'mkdir')
        await api.mutateFile(terminalID, { operation: 'mkdir', path: listing.data.path, name })
      if (operation.kind === 'rename')
        await api.mutateFile(terminalID, { operation: 'rename', path: operation.entry.path, name })
      if (operation.kind === 'chmod')
        await api.mutateFile(terminalID, {
          operation: 'chmod',
          path: operation.entry.path,
          mode: mode.toString(8).padStart(3, '0'),
        })
      if (operation.kind === 'delete') {
        const usage = operation.usage
        const entries = usage ? usage.files + usage.directories + usage.symlinks : 0
        await api.mutateFile(terminalID, {
          operation: 'delete',
          path: operation.entry.path,
          directory: operation.entry.directory,
          recursive: operation.entry.directory && entries > 0,
          expected: entries,
        })
      }
      await queries.invalidateQueries({ queryKey: ['files', terminalID] })
      setOperation(null)
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }
  async function copyPath(path: string) {
    try {
      await navigator.clipboard.writeText(path)
      notify(`已复制远端路径：${path}`)
    } catch {
      notify('浏览器未授予剪贴板权限。')
    }
  }
  const entries = useMemo(
    () =>
      sortEntries(
        listing.data?.entries.filter(
          (entry) =>
            (hidden || !entry.name.startsWith('.')) &&
            entry.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
        ) ?? [],
        sortKey,
        ascending,
      ),
    [listing.data, hidden, search, sortKey, ascending],
  )
  function sortButton(key: SortKey, label: string) {
    const selected = sortKey === key
    return (
      <button
        className={`sort-button ${selected ? 'selected' : ''}`}
        aria-label={`按${label}排序${selected ? (ascending ? '（升序）' : '（降序）') : ''}`}
        onClick={() => {
          if (selected) setAscending(!ascending)
          else {
            setSortKey(key)
            setAscending(key === 'name')
          }
        }}
      >
        {label}
        {selected && (ascending ? <ArrowUp size={11} /> : <ArrowDown size={11} />)}
      </button>
    )
  }
  const deleteUsage = operation?.kind === 'delete' ? operation.usage : undefined
  return (
    <aside className="file-panel" aria-label="SFTP 文件管理">
      <header className="panel-heading">
        <div>
          <Folder size={16} />
          <h2>远端文件</h2>
          <span className="subtle-badge">SFTP</span>
        </div>
        <button className="icon-button" aria-label="收起文件面板" onClick={onHide}>
          <X size={15} />
        </button>
      </header>
      <div className="file-controls">
        <form
          className="path-input"
          onSubmit={(event) => {
            event.preventDefault()
            navigate(address)
          }}
        >
          <button
            type="button"
            className="icon-button"
            aria-label="上级目录"
            disabled={!listing.data || listing.data.path === '/'}
            onClick={() => navigate(parentPath(listing.data?.path ?? '/'))}
          >
            <ArrowUp size={15} />
          </button>
          <input
            aria-label="远端路径"
            value={address}
            onChange={(event) => setAddress(event.target.value)}
            spellCheck={false}
          />
          <button
            className="icon-button"
            type="button"
            aria-label="刷新文件列表"
            disabled={listing.isFetching}
            onClick={() => void listing.refetch()}
          >
            <RefreshCw size={14} className={listing.isFetching ? 'spin' : ''} />
          </button>
        </form>
        <div className="file-actions">
          <button
            className="button secondary small"
            disabled={!listing.data}
            onClick={() => picker.current?.click()}
          >
            <Upload size={14} />
            上传
          </button>
          <button
            className="icon-button bordered"
            aria-label="上传文件夹"
            title="上传文件夹"
            disabled={!listing.data}
            onClick={() => folderPicker.current?.click()}
          >
            <FolderUp size={15} />
          </button>
          <button
            className="icon-button bordered"
            aria-label="新建文件夹"
            title="新建文件夹"
            disabled={!listing.data}
            onClick={() => void openOperation({ kind: 'mkdir' })}
          >
            <FolderPlus size={15} />
          </button>
          <button
            className={`follow-button ${follow ? 'enabled' : ''}`}
            disabled={!cwd}
            title={
              cwd
                ? `Shell 最近报告：${cwd}`
                : '需远端 Shell 发送与此主机匹配的 OSC 7 目录报告（设置 → Shell 集成）'
            }
            onClick={() => {
              setFollow(!follow)
              if (!follow && cwd) setDirectory(cwd)
            }}
          >
            <Link2 size={12} />
            {follow ? '跟随终端' : '手动目录'}
          </button>
        </div>
        <input
          ref={picker}
          type="file"
          multiple
          className="visually-hidden"
          aria-label="上传文件"
          onChange={(event) => {
            const files = Array.from(event.target.files ?? [])
            if (files.length && listing.data)
              onUpload(
                files.map((file) => ({ file, relativePath: file.name })),
                listing.data.path,
              )
            event.target.value = ''
          }}
        />
        <input
          ref={folderPicker}
          type="file"
          multiple
          className="visually-hidden"
          aria-label="上传文件夹"
          onChange={(event) => {
            const files = Array.from(event.target.files ?? [])
            if (files.length && listing.data) onUpload(fromFileList(files), listing.data.path)
            event.target.value = ''
          }}
        />
        <div className="file-search">
          <Search size={13} />
          <input
            aria-label="筛选文件"
            placeholder="筛选当前目录…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <label title="显示隐藏文件">
            <input
              type="checkbox"
              checked={hidden}
              onChange={(event) => setHidden(event.target.checked)}
            />
            <span>隐藏项</span>
          </label>
        </div>
        <div className="file-sort" role="group" aria-label="排序方式">
          {sortButton('name', '名称')}
          {sortButton('size', '大小')}
          {sortButton('modified', '修改时间')}
        </div>
      </div>
      <div
        className="file-list"
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes('Files')) event.preventDefault()
        }}
        onDrop={(event) => {
          event.preventDefault()
          const target = listing.data?.path
          if (!target) return
          fromDataTransfer(event.dataTransfer)
            .then((items) => onUpload(items, target))
            .catch((problem: unknown) =>
              notify(problem instanceof Error ? problem.message : '无法读取拖入的文件。'),
            )
        }}
      >
        {listing.isPending && (
          <div className="panel-empty">
            <LoaderCircle size={24} className="spin" />
            <p>正在读取远端目录…</p>
          </div>
        )}
        {listing.isError && (
          <div className="panel-empty">
            <Folder size={28} />
            <p>{messageOf(listing.error)}</p>
            <button className="button secondary small" onClick={() => void listing.refetch()}>
              重新读取
            </button>
          </div>
        )}
        {listing.isSuccess && entries.length === 0 && (
          <div className="panel-empty">
            <Folder size={30} />
            <p>{search ? '没有匹配的文件' : '目录是空的'}</p>
            <span>可以将文件或文件夹拖到这里上传</span>
          </div>
        )}
        {entries.map((entry, index) => (
          <div
            className={`file-row ${!entry.accessible ? 'unsupported' : ''}`}
            key={entry.path || `unsupported-${index}`}
          >
            <button
              className="file-open"
              disabled={!entry.accessible || entry.symlink}
              title={`${entry.path}\n${entry.mode} · ${shortDate(entry.modifiedAt)}`}
              onClick={() => (entry.directory ? navigate(entry.path) : onEdit(entry.path))}
            >
              {entry.symlink ? (
                <Link2 size={17} />
              ) : entry.directory ? (
                <Folder size={18} className="folder-icon" />
              ) : /\.(go|ts|tsx|js|json|yaml|yml|sh|py)$/.test(entry.name) ? (
                <FileCode2 size={17} />
              ) : (
                <FileText size={17} />
              )}
              <span>
                <strong>{entry.name}</strong>
                <small>
                  {entry.directory
                    ? `文件夹 · ${entry.mode}`
                    : entry.symlink
                      ? '符号链接（暂不操作）'
                      : `${formatBytes(entry.size)} · ${entry.mode} · ${shortDate(entry.modifiedAt)}`}
                </small>
              </span>
            </button>
            {entry.accessible && (
              <div className="file-row-actions">
                <button
                  className="icon-button"
                  title="复制远端路径"
                  aria-label={`复制路径 ${entry.name}`}
                  onClick={() => void copyPath(entry.path)}
                >
                  <ClipboardCopy size={12} />
                </button>
                {!entry.symlink &&
                  (entry.directory ? (
                    <a
                      href={api.archiveURL(terminalID, entry.path)}
                      download
                      className="icon-button"
                      title={`打包下载 ${entry.name}（zip，跳过符号链接）`}
                      aria-label={`打包下载 ${entry.name}`}
                    >
                      <FileArchive size={13} />
                    </a>
                  ) : (
                    <a
                      href={api.downloadURL(terminalID, entry.path)}
                      download
                      className="icon-button"
                      title={`下载 ${entry.name}`}
                      aria-label={`下载 ${entry.name}`}
                    >
                      <Download size={13} />
                    </a>
                  ))}
                {!entry.symlink && (
                  <>
                    <button
                      className="icon-button"
                      title="修改权限"
                      aria-label={`修改权限 ${entry.name}`}
                      onClick={() => void openOperation({ kind: 'chmod', entry })}
                    >
                      <KeyRound size={12} />
                    </button>
                    <button
                      className="icon-button"
                      title="重命名"
                      aria-label={`重命名 ${entry.name}`}
                      onClick={() => void openOperation({ kind: 'rename', entry })}
                    >
                      <Pencil size={12} />
                    </button>
                  </>
                )}
                <button
                  className="icon-button danger-hover"
                  title="删除"
                  aria-label={`删除 ${entry.name}`}
                  onClick={() => void openOperation({ kind: 'delete', entry })}
                >
                  <Trash2 size={12} />
                </button>
              </div>
            )}
          </div>
        ))}
      </div>
      <footer className="file-footer">
        <span>{listing.data ? `${entries.length} 个项目` : '未读取'}</span>
        <span>{follow ? '目录来自 Shell 报告' : '不假定为 Shell 当前目录'}</span>
      </footer>
      {operation && (
        <Modal
          title={
            operation.kind === 'mkdir'
              ? '新建文件夹'
              : operation.kind === 'rename'
                ? '重命名'
                : operation.kind === 'chmod'
                  ? '修改权限'
                  : '确认删除'
          }
          description={`${hostLabel} · ${operation.kind === 'mkdir' ? listing.data?.path : operation.entry.path}`}
          onClose={() => {
            if (!busy) setOperation(null)
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault()
              void mutate()
            }}
          >
            <div className="modal-body form-stack">
              {operation.kind === 'delete' &&
                (operation.entry.directory ? (
                  deleteUsage ? (
                    deleteUsage.files + deleteUsage.directories + deleteUsage.symlinks === 0 ? (
                      <p className="muted-copy">这是一个空目录。此操作无法撤销。</p>
                    ) : (
                      <div className="danger-summary">
                        <strong>
                          将递归删除 {deleteUsage.files} 个文件、{deleteUsage.directories}{' '}
                          个子目录和 {deleteUsage.symlinks} 个符号链接（
                          {formatBytes(deleteUsage.bytes)}）。
                        </strong>
                        <span>
                          符号链接只删除链接本身，不跟随到目标。删除前会再次核对数量，内容变化则不删除。此操作无法撤销。
                        </span>
                      </div>
                    )
                  ) : (
                    !error && (
                      <div className="attempt-progress">
                        <LoaderCircle size={15} className="spin" />
                        正在统计目录内容…
                      </div>
                    )
                  )
                ) : (
                  <p className="muted-copy">此操作无法撤销。</p>
                ))}
              {(operation.kind === 'mkdir' || operation.kind === 'rename') && (
                <label>
                  名称
                  <input
                    autoFocus
                    required
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={255}
                  />
                </label>
              )}
              {operation.kind === 'chmod' && (
                <>
                  <table className="permission-table">
                    <thead>
                      <tr>
                        <th />
                        <th>读</th>
                        <th>写</th>
                        <th>执行</th>
                      </tr>
                    </thead>
                    <tbody>
                      {permissionBits.map(([label, ...bits]) => (
                        <tr key={label}>
                          <th>{label}</th>
                          {bits.map((bit) => (
                            <td key={bit}>
                              <input
                                type="checkbox"
                                aria-label={`${label} ${bit}`}
                                checked={(mode & bit) !== 0}
                                onChange={(event) =>
                                  setMode(event.target.checked ? mode | bit : mode & ~bit)
                                }
                              />
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  <label>
                    八进制权限
                    <input
                      value={mode.toString(8).padStart(3, '0')}
                      pattern="[0-7]{3}"
                      maxLength={3}
                      onChange={(event) => {
                        const parsed = parseInt(event.target.value || '0', 8)
                        if (!Number.isNaN(parsed) && parsed <= 0o777) setMode(parsed)
                      }}
                    />
                  </label>
                  <p className="form-hint">只修改此项本身的权限，不递归，不修改特殊位。</p>
                </>
              )}
              {error && (
                <div className="inline-error" role="alert">
                  {error}
                </div>
              )}
            </div>
            <footer className="modal-footer">
              <button
                type="button"
                className="button secondary"
                disabled={busy}
                onClick={() => setOperation(null)}
              >
                取消
              </button>
              <button
                className={`button ${operation.kind === 'delete' ? 'danger' : 'primary'}`}
                disabled={
                  busy || (operation.kind === 'delete' && operation.entry.directory && !deleteUsage)
                }
              >
                {busy && <LoaderCircle size={15} className="spin" />}
                {operation.kind === 'delete' ? '永久删除' : '确认'}
              </button>
            </footer>
          </form>
        </Modal>
      )}
    </aside>
  )
}
