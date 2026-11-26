import { useEffect, useId, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  CornerDownLeft,
  LoaderCircle,
  Megaphone,
  Pencil,
  Plus,
  Search,
  TextCursorInput,
  Trash2,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { Snippet } from '../../shared/api/types'
import { useNotice } from '../../shared/ui/notices'
import {
  byteLength,
  commandByteLimit,
  commandLines,
  filterSnippets,
  terminalInput,
  trimTrailingNewlines,
  validateSnippet,
  type SnippetDraft,
} from './commands'
import type { CommandTarget } from './snippet-panel'

/** Stops implicit form submission when Enter only commits an IME composition. */
function holdComposingEnter(event: KeyboardEvent<HTMLFormElement>) {
  if (event.target instanceof HTMLTextAreaElement) return
  if (event.key === 'Enter' && (event.nativeEvent.isComposing || event.keyCode === 229))
    event.preventDefault()
}

const emptyDraft: SnippetDraft = { name: '', command: '', description: '' }

function targetLabel(target: CommandTarget) {
  return `${target.name} (${target.address})`
}

function SnippetForm({
  snippet,
  onUnsavedChange,
  onDone,
}: {
  snippet?: Snippet
  onUnsavedChange: (dirty: boolean) => void
  onDone: () => void
}) {
  const initial = useRef<SnippetDraft>(
    snippet
      ? { name: snippet.name, command: snippet.command, description: snippet.description }
      : emptyDraft,
  )
  const [draft, setDraft] = useState<SnippetDraft>(initial.current)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const queries = useQueryClient()
  const counterID = useId()
  const size = byteLength(trimTrailingNewlines(draft.command))
  const lines = commandLines(draft.command).length
  const dirty =
    draft.name !== initial.current.name ||
    draft.command !== initial.current.command ||
    draft.description !== initial.current.description

  useEffect(() => {
    onUnsavedChange(dirty)
    return () => onUnsavedChange(false)
  }, [dirty, onUnsavedChange])

  function change(field: keyof SnippetDraft, value: string) {
    setDraft((previous) => ({ ...previous, [field]: value }))
  }

  function cancel() {
    if (dirty && !window.confirm('放弃对命令片段的修改？')) return
    onDone()
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    const checked = validateSnippet(draft)
    if (checked.error !== undefined) {
      setError(checked.error)
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.saveSnippet(checked.input, snippet?.id)
      await queries.invalidateQueries({ queryKey: ['snippets'] })
      onUnsavedChange(false)
      onDone()
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form
      className="snippet-form"
      noValidate
      onKeyDown={holdComposingEnter}
      onSubmit={(event) => void save(event)}
    >
      <div className="modal-body form-stack">
        <h3 className="snippet-form-title">{snippet ? '编辑命令片段' : '新建命令片段'}</h3>
        <label>
          名称
          <input
            autoFocus
            required
            maxLength={80}
            placeholder="例如：查看磁盘占用"
            value={draft.name}
            onChange={(event) => change('name', event.target.value)}
          />
        </label>
        <div className="snippet-field">
          <label>
            命令
            <textarea
              required
              rows={7}
              spellCheck={false}
              autoCapitalize="none"
              autoCorrect="off"
              placeholder="df -h"
              aria-describedby={counterID}
              value={draft.command}
              onChange={(event) => change('command', event.target.value)}
            />
          </label>
          <span
            id={counterID}
            className={`snippet-counter ${size > commandByteLimit ? 'over' : ''}`}
          >
            {size} / {commandByteLimit} 字节
            {lines > 1 ? ` · ${lines} 行，执行时逐行按顺序运行` : ''}
          </span>
        </div>
        <label>
          说明（可选）
          <input
            maxLength={200}
            placeholder="何时使用、注意事项"
            value={draft.description}
            onChange={(event) => change('description', event.target.value)}
          />
        </label>
        <p className="form-hint">末尾的空行会被去掉；保存不会执行命令。</p>
        {error && (
          <div className="inline-error" role="alert">
            {error}
          </div>
        )}
      </div>
      <footer className="modal-footer">
        <button type="button" className="button secondary" disabled={busy} onClick={cancel}>
          取消
        </button>
        <button type="submit" className="button primary" disabled={busy}>
          {busy && <LoaderCircle size={16} className="spin" />}
          {snippet ? '保存修改' : '创建片段'}
        </button>
      </footer>
    </form>
  )
}

export function SnippetLibrary({
  snippets,
  loading,
  loadError,
  active,
  onUnsavedChange,
  onUseForBroadcast,
  onDone,
}: {
  snippets?: Snippet[]
  loading: boolean
  loadError?: Error
  active?: CommandTarget
  onUnsavedChange: (dirty: boolean) => void
  onUseForBroadcast: (command: string) => void
  onDone: () => void
}) {
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<Snippet | 'new' | null>(null)
  const [deleting, setDeleting] = useState<ReadonlySet<string>>(new Set())
  const queries = useQueryClient()
  const notice = useNotice()
  const visible = filterSnippets(snippets ?? [], query)
  const ready = !!active && active.ready()
  const unavailable = !active
    ? '没有活动终端：请先打开或切换到一个终端。'
    : !ready
      ? `${active.name} 的连接未就绪（正在连接、已断开或已结束）。`
      : undefined

  function insert(snippet: Snippet) {
    if (!active || !active.ready()) {
      notice(unavailable ?? '终端连接未就绪。')
      return
    }
    const lines = commandLines(snippet.command).length
    if (
      lines > 1 &&
      !window.confirm(
        [
          `「${snippet.name}」共 ${lines} 行。插入时每个换行都等于按下回车：`,
          `前 ${lines - 1} 行会立即在 ${targetLabel(active)} 上执行，只有最后一行留在提示符等你检查。`,
          ...(active.production ? ['注意：这是生产主机。'] : []),
          '',
          '继续插入？',
        ].join('\n'),
      )
    )
      return
    if (active.send(terminalInput(snippet.command, false))) {
      notice(`已插入到 ${active.name}，请在终端检查后自行按回车。`)
      onDone()
    } else notice(`未发送：${active.name} 的连接未就绪。`)
  }

  function execute(snippet: Snippet) {
    if (!active || !active.ready()) {
      notice(unavailable ?? '终端连接未就绪。')
      return
    }
    const lines = commandLines(snippet.command).length
    const message = [
      `在 ${targetLabel(active)} 上执行以下命令？`,
      ...(active.production ? ['注意：这是生产主机。'] : []),
      '',
      trimTrailingNewlines(snippet.command),
      '',
      lines > 1
        ? `共 ${lines} 行，会按顺序逐行输入并执行；前一行出错或启动了交互程序时，后续各行仍会被输入。`
        : '将在末尾自动按下回车。',
    ].join('\n')
    if (!window.confirm(message)) return
    if (active.send(terminalInput(snippet.command, true))) {
      notice(`已发送到 ${active.name}。`)
      onDone()
    } else notice(`未发送：${active.name} 的连接未就绪。`)
  }

  async function remove(snippet: Snippet) {
    if (deleting.has(snippet.id)) return
    if (!window.confirm(`删除命令片段「${snippet.name}」？此操作不可撤销。`)) return
    setDeleting((previous) => new Set(previous).add(snippet.id))
    try {
      await api.deleteSnippet(snippet.id)
      await queries.invalidateQueries({ queryKey: ['snippets'] })
    } catch (problem) {
      notice(`删除「${snippet.name}」失败：${messageOf(problem)}`)
    } finally {
      setDeleting((previous) => {
        const next = new Set(previous)
        next.delete(snippet.id)
        return next
      })
    }
  }

  if (editing)
    return (
      <SnippetForm
        snippet={editing === 'new' ? undefined : editing}
        onUnsavedChange={onUnsavedChange}
        onDone={() => setEditing(null)}
      />
    )

  return (
    <div className="modal-body snippet-library">
      <div className={`snippet-target ${ready ? '' : 'unavailable'}`} role="status">
        {active ? (
          <>
            <span>插入 / 执行目标：</span>
            <strong>{active.name}</strong>
            <code>{active.address}</code>
            {active.production && <span className="production-badge">生产</span>}
            {!ready && <span className="subtle-badge">未就绪</span>}
          </>
        ) : (
          <span>没有活动终端：插入和执行不可用，仍可管理片段或用于广播。</span>
        )}
      </div>
      <div className="snippet-toolbar">
        <label className="snippet-search">
          <Search size={14} />
          <span className="visually-hidden">搜索命令片段</span>
          <input
            type="search"
            placeholder="搜索名称或命令"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <button className="button small secondary" onClick={() => setEditing('new')}>
          <Plus size={14} />
          新建片段
        </button>
      </div>
      {loading ? (
        <p className="snippet-empty" role="status">
          <LoaderCircle size={15} className="spin" />
          正在读取命令片段…
        </p>
      ) : loadError ? (
        <div className="inline-error" role="alert">
          {messageOf(loadError)}
        </div>
      ) : visible.length === 0 ? (
        <p className="snippet-empty">
          {snippets?.length
            ? '没有匹配的片段。'
            : '还没有命令片段。保存常用命令后，可插入到当前终端或广播到多台主机。'}
        </p>
      ) : (
        <ul className="snippet-list" aria-label="命令片段">
          {visible.map((snippet) => {
            const lines = commandLines(snippet.command).length
            return (
              <li className="snippet-item" key={snippet.id}>
                <div className="snippet-item-head">
                  <strong>{snippet.name}</strong>
                  {lines > 1 && (
                    <span className="subtle-badge" title="执行时每一行按顺序运行">
                      {lines} 行
                    </span>
                  )}
                  <span className="snippet-item-tools">
                    <button
                      className="icon-button"
                      aria-label={`编辑片段 ${snippet.name}`}
                      title="编辑"
                      onClick={() => setEditing(snippet)}
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      className="icon-button danger-hover"
                      aria-label={`删除片段 ${snippet.name}`}
                      title="删除"
                      disabled={deleting.has(snippet.id)}
                      onClick={() => void remove(snippet)}
                    >
                      <Trash2 size={14} />
                    </button>
                  </span>
                </div>
                {snippet.description && (
                  <p className="snippet-description">{snippet.description}</p>
                )}
                <pre className="snippet-command">{snippet.command}</pre>
                <div className="snippet-item-actions">
                  <button
                    className="button small secondary"
                    disabled={!ready}
                    title={unavailable ?? '写入当前终端，不按回车；请检查后自行执行'}
                    aria-label={`插入片段 ${snippet.name} 到当前终端`}
                    onClick={() => insert(snippet)}
                  >
                    <TextCursorInput size={13} />
                    插入
                  </button>
                  <button
                    className="button small secondary"
                    disabled={!ready}
                    title={
                      unavailable ??
                      (lines > 1 ? `确认后逐行执行全部 ${lines} 行` : '确认后在当前终端执行')
                    }
                    aria-label={`在当前终端执行片段 ${snippet.name}`}
                    onClick={() => execute(snippet)}
                  >
                    <CornerDownLeft size={13} />
                    执行
                  </button>
                  <button
                    className="button small secondary"
                    title="切换到多主机广播并填入此命令，不会立即发送"
                    aria-label={`将片段 ${snippet.name} 用于广播`}
                    onClick={() => onUseForBroadcast(snippet.command)}
                  >
                    <Megaphone size={13} />
                    用于广播
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
