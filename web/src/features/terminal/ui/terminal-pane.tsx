import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { ArrowDown, ArrowUp, RotateCcw, Search, X } from 'lucide-react'
import type { TerminalController } from '../controller/terminal-controller'
import { useNotice } from '../../../shared/ui/notices'
import { fromDataTransfer, type UploadItem } from '../../../shared/lib/upload-items'

const states = {
  connecting: '正在建立通道',
  connected: '已连接',
  reconnecting: '正在重连 · 输入已暂停',
  exited: 'Shell 已结束',
  disconnected: '连接已断开',
}

export function TerminalPane({
  controller,
  title,
  production,
  focused,
  searchRequest,
  onFocus,
  onDrop,
  onReconnect,
}: {
  controller: TerminalController
  title: string
  production: boolean
  focused: boolean
  searchRequest: number
  onFocus: () => void
  onDrop: (items: UploadItem[]) => void
  onReconnect: () => void
}) {
  const container = useRef<HTMLDivElement>(null)
  const [find, setFind] = useState(false)
  const [query, setQuery] = useState('')
  const [dragging, setDragging] = useState(false)
  const notice = useNotice()
  const status = useSyncExternalStore(controller.subscribe, controller.snapshot)
  const exit = controller.exitState()
  useEffect(() => {
    if (!container.current) return
    controller.mount(container.current)
    return () => controller.unmount()
  }, [controller])
  useEffect(() => {
    if (focused) {
      controller.resize()
      controller.focus()
    }
  }, [focused, controller])
  useEffect(() => {
    if (searchRequest > 0 && focused) setFind(true)
  }, [searchRequest, focused])
  const ended = status === 'exited' || status === 'disconnected'
  let label: string = states[status]
  if (status === 'exited' && exit.code !== undefined) label = `Shell 已结束（退出码 ${exit.code}）`
  if (status === 'exited' && exit.signal) label = `Shell 被信号 ${exit.signal} 终止`
  return (
    <section
      className={`terminal-pane ${focused ? 'active focused' : ''} ${production ? 'production' : ''}`}
      aria-label={`终端 ${title}`}
      onMouseDown={onFocus}
      onDragOver={(event) => {
        if (!event.dataTransfer.types.includes('Files')) return
        event.preventDefault()
        setDragging(true)
      }}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node)) setDragging(false)
      }}
      onDrop={(event) => {
        event.preventDefault()
        setDragging(false)
        fromDataTransfer(event.dataTransfer)
          .then(onDrop)
          .catch((problem: unknown) =>
            notice(problem instanceof Error ? problem.message : '无法读取拖入的文件。'),
          )
      }}
    >
      <div className="terminal-toolbar">
        <span className={`status-dot ${status === 'connected' ? 'online' : 'waiting'}`} />
        <strong className="terminal-title">{title}</strong>
        {production && <span className="production-badge">生产</span>}
        <span>{label}</span>
        <span className="terminal-protocol">SSH / UTF-8</span>
        {ended && (
          <button
            className="icon-button"
            title="重新连接此主机（新的 Shell，不会恢复原会话）"
            aria-label={`重新连接 ${title}`}
            onClick={onReconnect}
          >
            <RotateCcw size={14} />
          </button>
        )}
        <button
          className="icon-button"
          title="搜索终端输出"
          aria-label="搜索终端输出"
          onClick={() => setFind(!find)}
        >
          <Search size={14} />
        </button>
      </div>
      {find && (
        <div className="terminal-find">
          <Search size={14} />
          <input
            aria-label="搜索终端"
            placeholder="搜索终端输出…"
            value={query}
            autoFocus
            onChange={(event) => {
              setQuery(event.target.value)
              controller.searchText(event.target.value)
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.nativeEvent.isComposing)
                controller.searchText(query, event.shiftKey)
              if (event.key === 'Escape') {
                setFind(false)
                controller.focus()
              }
            }}
          />
          <button
            className="icon-button"
            aria-label="上一处"
            onClick={() => controller.searchText(query, true)}
          >
            <ArrowUp size={14} />
          </button>
          <button
            className="icon-button"
            aria-label="下一处"
            onClick={() => controller.searchText(query)}
          >
            <ArrowDown size={14} />
          </button>
          <button
            className="icon-button"
            aria-label="关闭搜索"
            onClick={() => {
              setFind(false)
              controller.searchText('')
              controller.focus()
            }}
          >
            <X size={14} />
          </button>
        </div>
      )}
      <div ref={container} className="terminal-container" />
      {status !== 'connected' && status !== 'connecting' && (
        <div className="terminal-connection-note">
          {status === 'reconnecting'
            ? '保留现有画面，尝试恢复输出。不会重发命令。'
            : '此终端已停止接收输入。需要新 Shell 时请点击重新连接。'}
        </div>
      )}
      {dragging && (
        <div className="drop-overlay">
          <strong>上传到这台主机</strong>
          <span>松开后确认远端目录，支持文件夹，不会执行文件</span>
        </div>
      )}
    </section>
  )
}
