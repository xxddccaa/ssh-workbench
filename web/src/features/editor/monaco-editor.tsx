import { useEffect, useRef } from 'react'
import * as monaco from 'monaco-editor/editor/editor.api.js'
import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker'
import 'monaco-editor/languages/definitions/go/register.js'
import 'monaco-editor/languages/definitions/shell/register.js'
import 'monaco-editor/languages/definitions/yaml/register.js'
import 'monaco-editor/languages/definitions/markdown/register.js'
import 'monaco-editor/languages/definitions/javascript/register.js'
import 'monaco-editor/languages/definitions/python/register.js'

self.MonacoEnvironment = { getWorker: () => new EditorWorker() }

function languageOf(path: string) {
  const extension = path.split('.').pop() ?? ''
  const languages: Record<string, string> = {
    go: 'go',
    sh: 'shell',
    bash: 'shell',
    yaml: 'yaml',
    yml: 'yaml',
    md: 'markdown',
    js: 'javascript',
    ts: 'javascript',
    py: 'python',
  }
  return languages[extension] ?? 'plaintext'
}

export default function MonacoEditor({
  path,
  value,
  theme,
  readOnly,
  onChange,
  onSave,
}: {
  path: string
  value: string
  theme: 'dark' | 'light'
  readOnly: boolean
  onChange: (value: string) => void
  onSave: () => void
}) {
  const container = useRef<HTMLDivElement>(null)
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
  const callbacks = useRef({ onChange, onSave })
  callbacks.current = { onChange, onSave }
  useEffect(() => {
    if (!container.current) return
    const instance = monaco.editor.create(container.current, {
      value,
      language: languageOf(path),
      theme: theme === 'dark' ? 'vs-dark' : 'vs',
      readOnly,
      automaticLayout: true,
      fontSize: 13,
      fontFamily: '"SFMono-Regular", Consolas, monospace',
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      padding: { top: 18 },
      renderLineHighlight: 'gutter',
      tabSize: 2,
      fixedOverflowWidgets: true,
    })
    editor.current = instance
    const subscription = instance.onDidChangeModelContent(() =>
      callbacks.current.onChange(instance.getValue()),
    )
    instance.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () =>
      callbacks.current.onSave(),
    )
    return () => {
      subscription.dispose()
      instance.getModel()?.dispose()
      instance.dispose()
      editor.current = null
    }
  }, [path])
  useEffect(() => {
    if (editor.current && editor.current.getValue() !== value) editor.current.setValue(value)
  }, [value])
  useEffect(() => {
    monaco.editor.setTheme(theme === 'dark' ? 'vs-dark' : 'vs')
    editor.current?.updateOptions({ readOnly })
  }, [theme, readOnly])
  return <div ref={container} className="monaco-container" aria-label="文本编辑器" />
}
