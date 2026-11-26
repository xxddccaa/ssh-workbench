import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Megaphone, SquareTerminal } from 'lucide-react'
import { api } from '../../shared/api/client'
import { Modal } from '../../shared/ui/modal'
import { BroadcastComposer, type Prefill } from './broadcast-composer'
import { SnippetLibrary } from './snippet-library'
import './snippets.css'

export interface CommandTarget {
  id: string // terminal id
  name: string // host display name
  address: string // "user@host:port"
  production: boolean
  ready: () => boolean // true when the terminal socket can accept input right now
  send: (text: string) => boolean // writes text to that terminal; returns false if not sent
}

type Mode = 'snippets' | 'broadcast'

const tabs: { mode: Mode; label: string; icon: typeof SquareTerminal }[] = [
  { mode: 'snippets', label: '命令片段', icon: SquareTerminal },
  { mode: 'broadcast', label: '多主机广播', icon: Megaphone },
]

/** Re-renders periodically so ready() badges stay current; never moves focus. */
function useReadinessRefresh(interval = 1000) {
  const [, setTick] = useState(0)
  useEffect(() => {
    const timer = window.setInterval(() => setTick((tick) => tick + 1), interval)
    return () => window.clearInterval(timer)
  }, [interval])
}

export function SnippetPanel({
  mode,
  active,
  targets,
  onModeChange,
  onClose,
}: {
  mode: Mode
  active?: CommandTarget
  targets: CommandTarget[]
  onModeChange: (mode: Mode) => void
  onClose: () => void
}) {
  const snippets = useQuery({ queryKey: ['snippets'], queryFn: api.snippets })
  const [prefill, setPrefill] = useState<Prefill>()
  const unsaved = useRef(false)
  const tabRefs = useRef<Record<Mode, HTMLButtonElement | null>>({
    snippets: null,
    broadcast: null,
  })
  useReadinessRefresh()
  const trackUnsaved = useCallback((dirty: boolean) => {
    unsaved.current = dirty
  }, [])

  function close() {
    if (unsaved.current && !window.confirm('命令片段有未保存的修改，确定放弃并关闭？')) return
    onClose()
  }

  function tabKey(event: KeyboardEvent<HTMLButtonElement>) {
    const order = tabs.map((tab) => tab.mode)
    const index = order.indexOf(mode)
    const next =
      event.key === 'ArrowRight'
        ? order[(index + 1) % order.length]
        : event.key === 'ArrowLeft'
          ? order[(index - 1 + order.length) % order.length]
          : event.key === 'Home'
            ? order[0]
            : event.key === 'End'
              ? order[order.length - 1]
              : undefined
    if (!next) return
    event.preventDefault()
    onModeChange(next)
    tabRefs.current[next]?.focus()
  }

  return (
    <Modal
      wide
      title="命令片段与广播"
      description="片段只保存命令文本；任何发送到终端的操作都由你明确触发。"
      onClose={close}
    >
      <div className="snippet-tabs" role="tablist" aria-label="命令工具">
        {tabs.map(({ mode: tab, label, icon: Icon }) => (
          <button
            key={tab}
            ref={(element) => {
              tabRefs.current[tab] = element
            }}
            id={`command-tab-${tab}`}
            type="button"
            role="tab"
            aria-selected={mode === tab}
            aria-controls={`command-panel-${tab}`}
            tabIndex={mode === tab ? 0 : -1}
            className={`snippet-tab ${mode === tab ? 'selected' : ''}`}
            onClick={() => onModeChange(tab)}
            onKeyDown={tabKey}
          >
            <Icon size={15} />
            {label}
          </button>
        ))}
      </div>
      <div
        id="command-panel-snippets"
        role="tabpanel"
        aria-labelledby="command-tab-snippets"
        hidden={mode !== 'snippets'}
      >
        <SnippetLibrary
          snippets={snippets.data}
          loading={snippets.isPending}
          loadError={snippets.isError ? snippets.error : undefined}
          active={active}
          onUnsavedChange={trackUnsaved}
          onUseForBroadcast={(command) => {
            setPrefill((previous) => ({ command, serial: (previous?.serial ?? 0) + 1 }))
            onModeChange('broadcast')
          }}
          onDone={onClose}
        />
      </div>
      <div
        id="command-panel-broadcast"
        role="tabpanel"
        aria-labelledby="command-tab-broadcast"
        hidden={mode !== 'broadcast'}
      >
        <BroadcastComposer
          targets={targets}
          snippets={snippets.data ?? []}
          prefill={prefill}
          onClose={close}
        />
      </div>
    </Modal>
  )
}
