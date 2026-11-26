import { useId, useState, type ChangeEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  ChevronRight,
  CircleCheck,
  ClipboardPaste,
  FileText,
  FileUp,
  HardDrive,
  Info,
  LoaderCircle,
  ShieldCheck,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type {
  AuthState,
  ImportCandidate,
  ImportDiagnostic,
  ImportPreview,
  ImportResult,
} from '../../shared/api/types'
import { formatBytes } from '../../shared/lib/format'
import { Modal } from '../../shared/ui/modal'
import {
  allSelectable,
  contentLimit,
  contentProblem,
  defaultGroup,
  diagnosticLocation,
  formatEndpoint,
  formatJump,
  implicitJumps,
  levelLabels,
  previewSummary,
  selectable,
  selectedAliases,
  sortDiagnostics,
  toggled,
  worstLevel,
} from './import-selection'
import './imports.css'

type SourceChoice = 'local' | 'paste' | 'file'

interface LoadedFile {
  name: string
  size: number
  content: string
}

export function ImportDialog({
  auth,
  onClose,
  onImported,
}: {
  auth: AuthState
  onClose: () => void
  onImported: (result: ImportResult) => void
}) {
  const localFiles = !!auth.capabilities?.localFiles
  const [source, setSource] = useState<SourceChoice>(localFiles ? 'local' : 'paste')
  const [pasted, setPasted] = useState('')
  const [file, setFile] = useState<LoadedFile | null>(null)
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [selection, setSelection] = useState<Set<string>>(new Set())
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [group, setGroup] = useState('')
  const [result, setResult] = useState<ImportResult | null>(null)
  const [busy, setBusy] = useState<'preview' | 'apply' | null>(null)
  const [error, setError] = useState('')
  const queries = useQueryClient()

  const apiSource = source === 'local' ? 'local' : 'text'
  const content = source === 'paste' ? pasted : source === 'file' ? file?.content : undefined
  const local = source === 'local'
  const chosen = preview ? selectedAliases(selection, preview.candidates, local) : []
  const jumps = preview ? implicitJumps(selection, preview.candidates, local) : []

  function resetPreview() {
    setPreview(null)
    setSelection(new Set())
    setExpanded(new Set())
    setError('')
  }

  function choose(next: SourceChoice) {
    if (next === source) return
    setSource(next)
    resetPreview()
  }

  async function pick(event: ChangeEvent<HTMLInputElement>) {
    const input = event.currentTarget
    const selected = input.files?.[0]
    input.value = ''
    if (!selected) return
    resetPreview()
    setFile(null)
    if (selected.size > contentLimit) {
      setError('配置文件不得超过 1 MiB。')
      return
    }
    try {
      const text = await selected.text()
      setFile({ name: selected.name, size: selected.size, content: text })
    } catch {
      setError('无法读取所选文件。')
    }
  }

  async function parse() {
    if (busy) return
    if (source !== 'local') {
      const problem =
        source === 'file' && !file ? '请先选择配置文件。' : contentProblem(content ?? '')
      if (problem) {
        setError(problem)
        return
      }
    }
    setBusy('preview')
    setError('')
    try {
      const parsed = await api.previewImport(apiSource, content)
      setPreview(parsed)
      setSelection(allSelectable(parsed.candidates, local))
      setExpanded(new Set())
    } catch (problem) {
      setPreview(null)
      setError(messageOf(problem))
    } finally {
      setBusy(null)
    }
  }

  async function apply() {
    if (busy || chosen.length === 0) return
    setBusy('apply')
    setError('')
    try {
      const imported = await api.applyImport(apiSource, chosen, content, group.trim() || undefined)
      setResult(imported)
      await queries.invalidateQueries({ queryKey: ['hosts'] })
      onImported(imported)
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(null)
    }
  }

  const close = () => {
    if (!busy) onClose()
  }

  return (
    <Modal
      title="导入 SSH 配置"
      description="从 OpenSSH 配置中挑选主机，导入为 SSH Workbench 主机。不会执行配置中的任何命令。"
      onClose={close}
      wide
    >
      <div className="modal-body form-stack import-dialog">
        {result ? (
          <ImportSummary result={result} />
        ) : (
          <>
            <fieldset className="import-fieldset">
              <legend>配置来源</legend>
              <div className="choice-row">
                {localFiles && (
                  <button
                    type="button"
                    className={`choice ${source === 'local' ? 'selected' : ''}`}
                    aria-pressed={source === 'local'}
                    onClick={() => choose('local')}
                  >
                    <HardDrive size={15} />
                    本机 ~/.ssh/config
                  </button>
                )}
                <button
                  type="button"
                  className={`choice ${source === 'paste' ? 'selected' : ''}`}
                  aria-pressed={source === 'paste'}
                  onClick={() => choose('paste')}
                >
                  <ClipboardPaste size={15} />
                  粘贴配置内容
                </button>
                <button
                  type="button"
                  className={`choice ${source === 'file' ? 'selected' : ''}`}
                  aria-pressed={source === 'file'}
                  onClick={() => choose('file')}
                >
                  <FileUp size={15} />
                  选择文件
                </button>
              </div>
            </fieldset>
            <SourceExplanation auth={auth} source={source} localFiles={localFiles} />
            {source === 'paste' && (
              <label>
                配置内容
                <textarea
                  className="import-textarea"
                  spellCheck={false}
                  autoComplete="off"
                  rows={9}
                  placeholder={'Host web\n  HostName web.example.com\n  User deploy'}
                  value={pasted}
                  onChange={(event) => {
                    setPasted(event.target.value)
                    if (preview) resetPreview()
                  }}
                />
              </label>
            )}
            {source === 'file' && (
              <label className="key-picker">
                <FileText size={24} />
                <strong>{file ? file.name : '选择 SSH 配置文件'}</strong>
                <span>
                  {file
                    ? `${formatBytes(file.size)} · 已在浏览器中读取`
                    : '最大 1 MiB，在浏览器中读取后按粘贴内容处理'}
                </span>
                <input
                  type="file"
                  aria-label="选择 SSH 配置文件"
                  onChange={(event) => void pick(event)}
                />
              </label>
            )}
            <div className="import-actions">
              <button
                type="button"
                className="button secondary"
                disabled={!!busy}
                onClick={() => void parse()}
              >
                {busy === 'preview' && <LoaderCircle className="spin" size={15} />}
                {preview ? '重新解析' : '解析预览'}
              </button>
            </div>
            {error && !preview && (
              <div className="inline-error" role="alert">
                {error}
              </div>
            )}
            {preview && (
              <PreviewView
                preview={preview}
                source={source}
                selection={selection}
                expanded={expanded}
                jumps={jumps}
                group={group}
                onGroup={setGroup}
                onSelection={setSelection}
                onExpanded={setExpanded}
              />
            )}
            {error && preview && (
              <div className="inline-error" role="alert">
                {error}
              </div>
            )}
          </>
        )}
      </div>
      <footer className="modal-footer">
        {result ? (
          <button type="button" className="button primary" onClick={onClose}>
            完成
          </button>
        ) : (
          <>
            <button type="button" className="button secondary" disabled={!!busy} onClick={close}>
              取消
            </button>
            <button
              type="button"
              className="button primary"
              disabled={!preview || chosen.length === 0 || !!busy}
              onClick={() => void apply()}
            >
              {busy === 'apply' && <LoaderCircle className="spin" size={15} />}
              导入所选{chosen.length > 0 && `（${chosen.length}）`}
            </button>
          </>
        )}
      </footer>
    </Modal>
  )
}

function SourceExplanation({
  auth,
  source,
  localFiles,
}: {
  auth: AuthState
  source: SourceChoice
  localFiles: boolean
}) {
  const unavailable = !localFiles && (
    <>
      {auth.remote
        ? '远程模式下，服务不会读取所在机器的 ~/.ssh/config 或私钥文件，以免把服务器上的本机文件暴露给浏览器访问者。'
        : '当前服务没有开放本机文件读取。'}
      请粘贴配置内容或选择文件。
    </>
  )
  return (
    <div className="info-box">
      <Info size={19} />
      <span>
        {source === 'local' ? (
          <>
            由服务读取当前用户的 ~/.ssh/config（支持软链接 Include 与 ProxyCommand）。默认已在左侧栏直接列出，无需导入即可点击直连；如需编辑或标记，可在此导入保存。
          </>
        ) : (
          <>
            {unavailable}
            {unavailable && ' '}
            {source === 'file' ? '所选文件' : '粘贴的内容'}
            只在服务端解析，不会跟随 Include，也不会执行任何命令。其中 IdentityFile
            指向的文件不会被读取：使用私钥认证的主机，连接时需在浏览器中选择私钥，或关联已保存的凭据。
          </>
        )}
      </span>
    </div>
  )
}

function PreviewView({
  preview,
  source,
  selection,
  expanded,
  jumps,
  group,
  onGroup,
  onSelection,
  onExpanded,
}: {
  preview: ImportPreview
  source: SourceChoice
  selection: Set<string>
  expanded: Set<string>
  jumps: string[]
  group: string
  onGroup: (value: string) => void
  onSelection: (next: Set<string>) => void
  onExpanded: (next: Set<string>) => void
}) {
  const candidates = preview.candidates
  const local = source === 'local'
  const selectableCount = candidates.filter((candidate) => selectable(candidate, local)).length
  const chosen = selectedAliases(selection, candidates, local).length
  return (
    <section className="import-preview form-stack" aria-label="解析结果">
      <p className="import-summary-line" aria-live="polite">
        {previewSummary(preview, local)}
      </p>
      {preview.diagnostics.length > 0 && (
        <div className="import-file-diagnostics">
          <strong>配置文件说明</strong>
          <DiagnosticList diagnostics={preview.diagnostics} />
        </div>
      )}
      {candidates.length > 0 && (
        <>
          <div className="import-toolbar">
            <span>
              已选 {chosen} / {selectableCount}
            </span>
            <button
              type="button"
              className="text-button"
              disabled={selectableCount === 0 || chosen === selectableCount}
              onClick={() => onSelection(allSelectable(candidates, local))}
            >
              全选可导入
            </button>
            <button
              type="button"
              className="text-button"
              disabled={chosen === 0}
              onClick={() => onSelection(new Set())}
            >
              全不选
            </button>
          </div>
          <div className="import-table-wrap">
            <table className="import-table">
              <thead>
                <tr>
                  <th scope="col">
                    <span className="visually-hidden">选择</span>
                  </th>
                  <th scope="col">别名</th>
                  <th scope="col">地址</th>
                  <th scope="col">跳板</th>
                  <th scope="col">私钥文件</th>
                  <th scope="col">状态</th>
                </tr>
              </thead>
              <tbody>
                {candidates.map((candidate, index) => (
                  <CandidateRows
                    key={`${candidate.alias}-${index}`}
                    candidate={candidate}
                    local={local}
                    checked={selectable(candidate, local) && selection.has(candidate.alias)}
                    open={expanded.has(candidate.alias)}
                    onCheck={(on) => onSelection(toggled(selection, candidate.alias, on))}
                    onOpen={(on) => onExpanded(toggled(expanded, candidate.alias, on))}
                  />
                ))}
              </tbody>
            </table>
          </div>
          <label className="import-group">
            分组名称
            <input
              maxLength={80}
              placeholder={defaultGroup}
              value={group}
              onChange={(event) => onGroup(event.target.value)}
            />
          </label>
          <div className="info-box">
            <ShieldCheck size={19} />
            <span>
              所选主机用到的 ProxyJump
              跳板会一并导入；已存在同名或同地址的主机时直接关联，不会重复创建。
              {jumps.length > 0 && `本次将一并处理：${jumps.join('、')}。`}
              导入不会信任任何主机密钥，首次连接时仍需确认指纹。
            </span>
          </div>
        </>
      )}
    </section>
  )
}

function CandidateRows({
  candidate,
  local,
  checked,
  open,
  onCheck,
  onOpen,
}: {
  candidate: ImportCandidate
  local: boolean
  checked: boolean
  open: boolean
  onCheck: (on: boolean) => void
  onOpen: (on: boolean) => void
}) {
  const canSelect = selectable(candidate, local)
  const level = worstLevel(candidate.diagnostics)
  const count = candidate.diagnostics.length
  const detailID = useId()
  const disabledReason = candidate.existing
    ? '已存在同名或同地址的主机'
    : !candidate.importable
      ? '包含不支持的设置，请查看说明'
      : !canSelect
        ? '缺少 User，无法确定登录用户名'
        : undefined
  return (
    <>
      <tr className={canSelect ? undefined : 'import-row-disabled'}>
        <td>
          <input
            type="checkbox"
            aria-label={`选择 ${candidate.alias}`}
            title={disabledReason}
            disabled={!canSelect}
            checked={checked}
            onChange={(event) => onCheck(event.target.checked)}
          />
        </td>
        <th scope="row" className="import-alias">
          {candidate.alias}
        </th>
        <td>
          <span className="import-mono">
            {formatEndpoint(candidate.user, candidate.hostname, candidate.port)}
          </span>
          {!candidate.user && local && (
            <span className="import-note">未设置 User，将使用服务所在机器的当前用户</span>
          )}
        </td>
        <td className="import-mono">{candidate.jump ? formatJump(candidate.jump) : '—'}</td>
        <td className="import-mono">
          {candidate.identityFiles?.length
            ? candidate.identityFiles.map((path) => <div key={path}>{path}</div>)
            : '—'}
        </td>
        <td>
          <div className="import-badges">
            {candidate.existing && <span className="subtle-badge">已存在</span>}
            {!candidate.importable && (
              <span className="subtle-badge import-badge-blocked">不可导入</span>
            )}
            {!candidate.user && !local && candidate.importable && !candidate.existing && (
              <span className="subtle-badge import-badge-warning">缺少 User</span>
            )}
            {count > 0 && (
              <button
                type="button"
                className={`import-toggle ${level ?? ''}`}
                aria-expanded={open}
                aria-controls={detailID}
                onClick={() => onOpen(!open)}
              >
                {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
                {count} 条说明
              </button>
            )}
          </div>
        </td>
      </tr>
      {count > 0 && open && (
        <tr className="import-detail-row" id={detailID}>
          <td />
          <td colSpan={5}>
            <DiagnosticList diagnostics={candidate.diagnostics} />
          </td>
        </tr>
      )}
    </>
  )
}

function DiagnosticList({ diagnostics }: { diagnostics: ImportDiagnostic[] }) {
  return (
    <ul className="import-diagnostics">
      {sortDiagnostics(diagnostics).map((diagnostic, index) => {
        const location = diagnosticLocation(diagnostic)
        return (
          <li key={index} className={`import-diagnostic ${diagnostic.level}`}>
            <span className="import-level">{levelLabels[diagnostic.level]}</span>
            <span>
              {diagnostic.directive && <code>{diagnostic.directive}</code>} {diagnostic.message}
              {location && <small>{location}</small>}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

function ImportSummary({ result }: { result: ImportResult }) {
  return (
    <section className="import-result form-stack" aria-live="polite">
      <div className="setting-detail">
        <CircleCheck size={20} />
        <div>
          <strong>
            已导入 {result.created.length} 个主机
            {result.skipped.length > 0 && `，跳过 ${result.skipped.length} 个`}
          </strong>
          <p>导入的主机尚未信任任何主机密钥，首次连接时会请你确认指纹。</p>
        </div>
      </div>
      {result.created.length > 0 && (
        <div>
          <h3 className="import-result-heading">已创建</h3>
          <ul className="import-result-list">
            {result.created.map((host) => (
              <li key={host.id}>
                <strong>{host.name}</strong>
                <span className="import-mono">
                  {formatEndpoint(host.username, host.hostname, host.port)}
                </span>
                {host.group && <span className="subtle-badge">{host.group}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
      {result.skipped.length > 0 && (
        <div>
          <h3 className="import-result-heading">已跳过</h3>
          <ul className="import-result-list">
            {result.skipped.map((skip, index) => (
              <li key={`${skip.alias}-${index}`}>
                <strong>{skip.alias}</strong>
                <span>{skip.reason}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
