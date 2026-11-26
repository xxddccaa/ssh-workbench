import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { ExternalLink, FileCode2, LoaderCircle, RotateCcw, Save, X } from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { RemoteDocument } from '../../shared/api/types'

const MonacoEditor = lazy(() => import('./monaco-editor'))

export type DocumentSource =
  { kind: 'remote'; terminalID: string; path: string } | { kind: 'local'; documentID: string }

function loadFrom(source: DocumentSource, signal?: AbortSignal): Promise<RemoteDocument> {
  return source.kind === 'remote'
    ? api.document(source.terminalID, source.path, signal)
    : api.localDocument(source.documentID, signal)
}

function saveTo(source: DocumentSource, document: RemoteDocument): Promise<RemoteDocument> {
  if (source.kind === 'remote') return api.saveDocument(source.terminalID, document)
  const { path, content, version, lineEnding, bom } = document
  return api.saveLocalDocument(source.documentID, { path, content, version, lineEnding, bom })
}

export function EditorPanel({
  source,
  theme,
  standalone = false,
  onClose,
  onDirtyChange,
}: {
  source: DocumentSource
  theme: 'dark' | 'light'
  standalone?: boolean
  onClose?: () => void
  onDirtyChange?: (dirty: boolean) => void
}) {
  const [document, setDocument] = useState<RemoteDocument | null>(null)
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saved, setSaved] = useState(false)
  const normalized = (value: string) => value.replace(/\r\n/g, '\n')
  const sourceKey =
    source.kind === 'remote' ? `${source.terminalID}:${source.path}` : `local:${source.documentID}`
  const path = source.kind === 'remote' ? source.path : (document?.path ?? '本机文件')
  const dirty = document !== null && normalized(content) !== normalized(document.content)
  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true)
      setError('')
      try {
        const loaded = await loadFrom(source, signal)
        setDocument(loaded)
        setContent(loaded.content)
        setSaved(false)
      } catch (problem) {
        if (!signal?.aborted) setError(messageOf(problem))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [sourceKey],
  )
  useEffect(() => {
    const abort = new AbortController()
    void load(abort.signal)
    return () => abort.abort()
  }, [load])
  useEffect(() => {
    const preventLoss = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', preventLoss)
    return () => window.removeEventListener('beforeunload', preventLoss)
  }, [dirty])
  async function save() {
    if (!document || saving || !dirty) return
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      const next = await saveTo(source, { ...document, content })
      setDocument(next)
      setContent(next.content)
      setSaved(true)
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setSaving(false)
    }
  }
  function close() {
    if (!dirty || window.confirm('文件有未保存的修改，确认放弃？')) onClose?.()
  }
  function newWindow() {
    if (source.kind !== 'remote') return
    const target = new URL('/editor', location.href)
    target.searchParams.set('terminal', source.terminalID)
    target.searchParams.set('path', source.path)
    window.open(target, '_blank', 'noopener,noreferrer')
  }
  return (
    <section
      className={`editor-panel ${standalone ? 'standalone-editor' : ''}`}
      aria-label="远端文本编辑器"
    >
      <header className="editor-heading">
        <FileCode2 size={16} />
        <div>
          <strong>
            {path.split('/').pop()}
            {dirty && <span className="dirty-dot" />}
          </strong>
          <span title={path}>{path}</span>
        </div>
        <button
          className="icon-button"
          title="重新读取"
          aria-label="重新读取文件"
          disabled={loading || saving}
          onClick={() => {
            if (!dirty || window.confirm('放弃未保存的修改并重新读取？')) void load()
          }}
        >
          <RotateCcw size={15} />
        </button>
        {!standalone && source.kind === 'remote' && (
          <button
            className="icon-button"
            title="新窗口编辑"
            aria-label="新窗口编辑"
            onClick={newWindow}
          >
            <ExternalLink size={15} />
          </button>
        )}
        <button
          className="button primary small"
          disabled={!dirty || saving || document?.lineEnding === 'mixed'}
          onClick={() => void save()}
        >
          {saving ? <LoaderCircle size={14} className="spin" /> : <Save size={14} />}保存
        </button>
        {onClose && (
          <button className="icon-button" aria-label="关闭编辑器" onClick={close}>
            <X size={16} />
          </button>
        )}
      </header>
      {error && (
        <div className="editor-error" role="alert">
          {error}
        </div>
      )}
      {loading ? (
        <div className="editor-loading">
          <LoaderCircle size={25} className="spin" />
          <p>读取文本…</p>
        </div>
      ) : document ? (
        <Suspense fallback={<div className="editor-loading">加载本地编辑器资源…</div>}>
          <MonacoEditor
            path={path}
            value={content}
            theme={theme}
            readOnly={saving || document.lineEnding === 'mixed'}
            onChange={(value) => {
              setContent(value)
              setSaved(false)
            }}
            onSave={() => void save()}
          />
        </Suspense>
      ) : (
        <div className="editor-loading">
          <FileCode2 size={32} />
          <p>文件未加载，不会进行任何写入。</p>
        </div>
      )}
      <footer className="editor-footer">
        <span>
          {saved
            ? source.kind === 'local'
              ? '已保存到本机文件'
              : '已保存到远端'
            : dirty
              ? '有未保存的修改'
              : '先检测冲突，再原子替换'}
        </span>
        <span>
          UTF-8{document?.bom ? ' BOM' : ''} · {document?.lineEnding.toUpperCase() ?? '—'} · ≤ 2 MiB
        </span>
      </footer>
    </section>
  )
}
