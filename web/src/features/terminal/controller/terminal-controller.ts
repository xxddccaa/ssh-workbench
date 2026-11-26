import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import {
  binaryInput,
  inputFrames,
  isComposing,
  outputFrame,
  reportedDirectory,
} from '../../../shared/protocol/terminal'

export type TerminalStatus = 'connecting' | 'connected' | 'reconnecting' | 'exited' | 'disconnected'

export interface TerminalOptions {
  fontSize: number
  scrollback: number
  cursorStyle: 'bar' | 'block' | 'underline'
}

export interface ExitState {
  code?: number
  signal?: string
}

const darkTheme = {
  background: '#101419',
  foreground: '#d5dce5',
  cursor: '#83d8b7',
  selectionBackground: '#264f49',
  black: '#28303b',
  red: '#ef8888',
  green: '#9bd6ad',
  yellow: '#e4c88c',
  blue: '#8bb8ef',
  magenta: '#b6a0d9',
  cyan: '#82c9d0',
  white: '#d5dce5',
  brightBlack: '#748297',
  brightRed: '#ffb3b3',
  brightGreen: '#b4f0bd',
  brightYellow: '#f7dfae',
  brightBlue: '#b2d3ff',
  brightMagenta: '#d4baff',
  brightCyan: '#a4e5e9',
  brightWhite: '#f5f7fb',
}
const lightTheme = {
  ...darkTheme,
  background: '#fafbfc',
  foreground: '#263341',
  cursor: '#19775b',
  selectionBackground: '#cbeadf',
  black: '#35404c',
  white: '#536174',
  brightWhite: '#293443',
}

function openLink(value: string) {
  try {
    const target = new URL(value)
    if (target.protocol === 'https:' || target.protocol === 'http:')
      window.open(target.href, '_blank', 'noopener,noreferrer')
  } catch {
    return
  }
}

export class TerminalController {
  readonly terminal: Terminal
  private readonly fit = new FitAddon()
  private readonly search = new SearchAddon()
  private readonly surface = document.createElement('div')
  private readonly listeners = new Set<() => void>()
  private readonly encoder = new TextEncoder()
  private socket: WebSocket | null = null
  private resizeObserver?: ResizeObserver
  private resizeFrame = 0
  private retryTimer = 0
  private retryCount = 0
  private generation = 0
  private appliedOffset = 0n
  private queuedOffset = 0n
  private pendingWrites = 0
  private opened = false
  private disposed = false
  private status: TerminalStatus = 'connecting'
  private exit: ExitState = {}
  private lastInputNotice = 0

  constructor(
    readonly id: string,
    readonly hostname: string,
    private callbacks: {
      directory: (path: string) => void
      notice: (message: string) => void
      shortcut: (event: KeyboardEvent) => boolean
      focused?: () => void
    },
    theme: 'dark' | 'light',
    options: TerminalOptions,
  ) {
    this.surface.className = 'terminal-surface'
    this.terminal = new Terminal({
      allowProposedApi: true,
      fontFamily: 'Menlo, Monaco, Consolas, "DejaVu Sans Mono", "Liberation Mono", monospace',
      fontSize: options.fontSize,
      lineHeight: 1.25,
      cursorBlink: !window.matchMedia('(prefers-reduced-motion: reduce)').matches,
      cursorStyle: options.cursorStyle,
      scrollback: options.scrollback,
      theme: theme === 'dark' ? darkTheme : lightTheme,
      rightClickSelectsWord: true,
      linkHandler: { activate: (_event, value) => openLink(value), allowNonHttpProtocols: false },
    })
    this.terminal.loadAddon(this.fit)
    this.terminal.loadAddon(this.search)
    this.terminal.loadAddon(new Unicode11Addon())
    this.terminal.unicode.activeVersion = '11'
    this.terminal.loadAddon(new WebLinksAddon((_event, value) => openLink(value)))
    this.terminal.parser.registerOscHandler(7, (data) => {
      const directory = reportedDirectory(data, this.hostname)
      if (directory) this.callbacks.directory(directory)
      return true
    })
    this.terminal.parser.registerOscHandler(52, () => true)
    this.terminal.onData((data) => this.send(this.encoder.encode(data)))
    this.terminal.onBinary((data) => this.send(binaryInput(data)))
    this.terminal.onResize(({ rows, cols }) => this.control({ type: 'resize', rows, cols }))
    this.terminal.attachCustomKeyEventHandler((event) => {
      if (isComposing(event) || event.type !== 'keydown') return true
      // Application shortcuts are handled by the workbench; keep them out of
      // the shell so one key press does not also type into the terminal.
      if (this.callbacks.shortcut(event)) return false
      if ((event.metaKey || (event.ctrlKey && event.shiftKey)) && event.key.toLowerCase() === 'c') {
        if (this.terminal.hasSelection()) void this.copySelection()
        return false
      }
      return true
    })
    this.surface.addEventListener(
      'paste',
      (event) => {
        const text = event.clipboardData?.getData('text/plain')
        if (text !== undefined) {
          event.preventDefault()
          event.stopImmediatePropagation()
          this.paste(text)
        }
      },
      true,
    )
  }

  /** Called by the workbench when the pane gains focus. */
  private trackFocus = () => this.callbacks.focused?.()

  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }
  snapshot = () => this.status
  exitState = () => this.exit

  private setStatus(status: TerminalStatus) {
    if (this.status === status) return
    this.status = status
    for (const listener of this.listeners) listener()
  }

  mount(container: HTMLElement) {
    if (this.disposed) return
    container.append(this.surface)
    if (!this.opened) {
      this.terminal.open(this.surface)
      this.opened = true
      this.terminal.textarea?.addEventListener('focus', this.trackFocus)
      this.connect()
    }
    this.resizeObserver?.disconnect()
    this.resizeObserver = new ResizeObserver(() => this.resize())
    this.resizeObserver.observe(container)
    void document.fonts.ready.then(() => this.resize())
    this.resize()
  }

  unmount() {
    this.resizeObserver?.disconnect()
    this.surface.remove()
  }

  resize() {
    cancelAnimationFrame(this.resizeFrame)
    this.resizeFrame = requestAnimationFrame(() => {
      if (this.disposed || this.surface.clientWidth < 40 || this.surface.clientHeight < 30) return
      this.fit.fit()
    })
  }

  focus() {
    if (!this.disposed) this.terminal.focus()
  }
  setTheme(theme: 'dark' | 'light') {
    this.terminal.options.theme = theme === 'dark' ? darkTheme : lightTheme
  }
  setOptions(options: TerminalOptions) {
    const current = this.terminal.options
    if (current.fontSize !== options.fontSize) current.fontSize = options.fontSize
    if (current.scrollback !== options.scrollback) current.scrollback = options.scrollback
    if (current.cursorStyle !== options.cursorStyle) current.cursorStyle = options.cursorStyle
    this.resize()
  }

  /** True when input typed now would reach the remote shell. */
  ready() {
    return (
      !this.disposed && this.socket?.readyState === WebSocket.OPEN && this.status === 'connected'
    )
  }

  /**
   * Sends text as if pasted, without the multi-line confirmation, for
   * explicit snippet and broadcast actions the user already confirmed.
   * Returns false when nothing was sent; it never queues or retries.
   */
  sendText(text: string): boolean {
    if (!this.ready()) return false
    const bytes = this.encoder.encode(text)
    if (bytes.length + (this.socket?.bufferedAmount ?? 0) > 1024 * 1024) return false
    for (const frame of inputFrames(bytes)) this.socket?.send(frame)
    return true
  }
  searchText(text: string, backwards = false) {
    if (!text) {
      this.search.clearDecorations()
      return
    }
    if (backwards) this.search.findPrevious(text)
    else this.search.findNext(text)
  }

  async copySelection() {
    try {
      await navigator.clipboard.writeText(this.terminal.getSelection())
    } catch {
      this.callbacks.notice('浏览器未授予剪贴板权限，请使用系统复制快捷键。')
    }
  }

  paste(text: string) {
    if (
      (text.includes('\n') || text.includes('\r')) &&
      !window.confirm('粘贴包含多行内容，可能立即执行多条命令。确认粘贴到当前主机？')
    )
      return
    this.terminal.paste(text)
    this.focus()
  }

  private send(bytes: Uint8Array) {
    if (this.socket?.readyState !== WebSocket.OPEN || this.status !== 'connected') {
      if (Date.now() - this.lastInputNotice > 3000) {
        this.lastInputNotice = Date.now()
        this.callbacks.notice('连接尚未就绪，本次输入未发送，也不会自动重放。')
      }
      return
    }
    if (bytes.length + this.socket.bufferedAmount > 1024 * 1024) {
      this.callbacks.notice('输入缓冲已满或粘贴超过 1 MiB，请稍后分批输入。')
      return
    }
    for (const frame of inputFrames(bytes)) this.socket.send(frame)
  }

  private control(message: object) {
    if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(message))
  }

  private connect() {
    if (this.disposed) return
    if (this.pendingWrites > 0) {
      this.retryTimer = window.setTimeout(() => this.connect(), 100)
      return
    }
    this.queuedOffset = this.appliedOffset
    const generation = ++this.generation
    const address = new URL(`/ws/terminals/${encodeURIComponent(this.id)}`, location.href)
    address.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    address.searchParams.set('offset', this.appliedOffset.toString())
    const socket = new WebSocket(address)
    socket.binaryType = 'arraybuffer'
    this.socket = socket
    socket.onopen = () => {
      if (generation !== this.generation || this.disposed) {
        socket.close()
        return
      }
      this.retryCount = 0
      this.setStatus('connected')
      this.control({ type: 'resize', rows: this.terminal.rows, cols: this.terminal.cols })
    }
    socket.onmessage = (event: MessageEvent<string | ArrayBuffer>) => {
      if (this.disposed || generation !== this.generation) return
      try {
        if (typeof event.data === 'string') {
          const message: unknown = JSON.parse(event.data)
          if (typeof message !== 'object' || message === null || !('type' in message))
            throw new Error('控制消息无效。')
          if (message.type === 'hello' && (!('version' in message) || message.version !== 1))
            throw new Error('终端协议版本不兼容。')
          if (message.type === 'state') {
            this.exit = {
              code:
                'exitCode' in message && typeof message.exitCode === 'number'
                  ? message.exitCode
                  : undefined,
              signal:
                'exitSignal' in message && typeof message.exitSignal === 'string'
                  ? message.exitSignal
                  : undefined,
            }
            this.setStatus(
              'status' in message && message.status === 'disconnected' ? 'disconnected' : 'exited',
            )
          }
          return
        }
        const frame = outputFrame(event.data)
        const end = frame.offset + BigInt(frame.bytes.length)
        if (end <= this.queuedOffset) return
        if (frame.offset > this.queuedOffset) throw new Error('终端输出出现间断，请重新连接。')
        const skip = Number(this.queuedOffset - frame.offset)
        this.queuedOffset = end
        this.pendingWrites++
        this.terminal.write(frame.bytes.subarray(skip), () => {
          this.pendingWrites--
          if (end > this.appliedOffset) this.appliedOffset = end
          if (!this.disposed && generation === this.generation)
            this.control({ type: 'ack', offset: end.toString() })
        })
      } catch (error) {
        this.callbacks.notice(error instanceof Error ? error.message : '终端通道错误。')
        socket.close(1002, 'protocol error')
      }
    }
    socket.onclose = (event) => {
      if (this.disposed || generation !== this.generation) return
      if ([1000, 1002, 1008, 4001].includes(event.code) || this.status === 'exited') {
        this.setStatus(this.status === 'exited' || event.code === 1000 ? 'exited' : 'disconnected')
        return
      }
      if (this.retryCount >= 7) {
        this.setStatus('disconnected')
        return
      }
      this.setStatus('reconnecting')
      this.retryTimer = window.setTimeout(
        () => this.connect(),
        Math.min(500 * 2 ** this.retryCount++, 5000),
      )
    }
    socket.onerror = () => undefined
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.generation++
    clearTimeout(this.retryTimer)
    cancelAnimationFrame(this.resizeFrame)
    this.resizeObserver?.disconnect()
    this.terminal.textarea?.removeEventListener('focus', this.trackFocus)
    this.socket?.close(1000, 'view closed')
    this.listeners.clear()
    this.terminal.dispose()
    this.surface.remove()
  }
}
