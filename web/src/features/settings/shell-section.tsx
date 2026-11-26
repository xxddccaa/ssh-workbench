import { Check, Copy, FolderSync, Info } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { SettingsHeading } from './settings-heading'
import { shellSnippets, type ShellSnippet } from './shell-integration'

export function ShellSection() {
  return (
    <>
      <SettingsHeading title="Shell 集成">
        让文件面板跟随终端的当前目录。需要你自己在远端 shell 配置中加入下面的片段。
      </SettingsHeading>
      <div className="setting-detail">
        <FolderSync size={20} />
        <div>
          <strong>通过 OSC 7 报告目录</strong>
          <p>
            加入片段后，shell 每次显示提示符时会向终端发送一条 OSC 7
            目录报告。在文件面板中切换为“跟随终端”，面板就会跟着目录变化。SSH Workbench
            不会修改远端的 rc 文件，也不会向终端注入任何命令。
          </p>
        </div>
      </div>
      <div className="info-box">
        <Info size={19} />
        <span>
          报告中的主机名必须与 SSH Workbench
          中填写的主机地址一致（不区分大小写），否则报告会被忽略，以免其他来源的输出改变文件面板的目录。片段默认使用
          shell 自己的主机名（Bash 为 <code>$HOSTNAME</code>，Zsh 为 <code>$HOST</code>
          ）。如果你填写的是 IP、完整域名或其他名字，请把片段里的 <code>"$HOSTNAME"</code> 或{' '}
          <code>"$HOST"</code> 改成该地址，例如 <code>"10.0.0.5"</code>；IPv6 地址需加方括号。
        </span>
      </div>
      {shellSnippets.map((snippet) => (
        <SnippetBlock key={snippet.kind} snippet={snippet} />
      ))}
      <p className="form-hint">
        片段只在交互式 shell 显示提示符时输出，不影响 scp、rsync 等非交互会话。保存后重新登录或执行{' '}
        <code>source</code> 使其生效。
      </p>
    </>
  )
}

function SnippetBlock({ snippet }: { snippet: ShellSnippet }) {
  const code = useRef<HTMLPreElement>(null)
  const [status, setStatus] = useState<'copied' | 'failed' | null>(null)

  useEffect(() => {
    if (status !== 'copied') return
    const timer = window.setTimeout(() => setStatus(null), 2000)
    return () => window.clearTimeout(timer)
  }, [status])

  async function copy() {
    try {
      if (!navigator.clipboard) throw new Error('clipboard unavailable')
      await navigator.clipboard.writeText(snippet.code)
      setStatus('copied')
    } catch {
      // Select the text so a manual copy is one shortcut away.
      const element = code.current
      const selection = window.getSelection()
      if (element && selection) {
        const range = document.createRange()
        range.selectNodeContents(element)
        selection.removeAllRanges()
        selection.addRange(range)
        element.focus()
      }
      setStatus('failed')
    }
  }

  const titleID = `shell-snippet-${snippet.kind}`
  const copied = status === 'copied'
  return (
    <section className="settings-snippet" aria-labelledby={titleID}>
      <header>
        <strong id={titleID}>
          {snippet.label}
          <span>
            添加到远端 <code>{snippet.rcFile}</code> 末尾 · 使用 <code>{snippet.hook}</code>
          </span>
        </strong>
        <button
          type="button"
          className="button secondary small"
          aria-label={`${copied ? '已复制' : '复制'} ${snippet.label} 片段`}
          onClick={() => void copy()}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? '已复制' : '复制'}
        </button>
      </header>
      {status === 'failed' && (
        <p className="settings-snippet-error" role="alert">
          无法访问剪贴板，已选中片段文本，请按 Ctrl/⌘+C 手动复制。
        </p>
      )}
      <pre ref={code} tabIndex={0} aria-label={`${snippet.label} 片段`}>
        <code>{snippet.code}</code>
      </pre>
    </section>
  )
}
