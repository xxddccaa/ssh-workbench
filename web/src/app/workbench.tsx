import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDownToLine,
  Columns2,
  Command,
  Files,
  Grid2x2,
  LoaderCircle,
  Megaphone,
  Moon,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  RotateCcw,
  Rows2,
  Server,
  Settings2,
  Square,
  Sun,
  Terminal,
  X,
} from 'lucide-react'
import { api, messageOf } from '../shared/api/client'
import type {
  Attempt,
  AuthState,
  HostProfile,
  LaunchResult,
  Preferences,
  QuickConnectTarget,
  TerminalInfo,
  Workspace,
} from '../shared/api/types'
import { useNotice } from '../shared/ui/notices'
import { actionFor, bindings, describe, type ActionID } from '../shared/lib/keybindings'
import type { UploadItem } from '../shared/lib/upload-items'
import { HostSidebar } from '../features/hosts/host-sidebar'
import { HostDialog } from '../features/hosts/host-dialog'
import {
  isLocalHost,
  localHostProfile,
  quickTargetToProfile,
  sshConfigToProfile,
} from '../features/hosts/host-helpers'
import { ConnectionDialog } from '../features/connect/connection-dialog'
import { useAttempt } from '../features/connect/use-attempt'
import { TerminalController } from '../features/terminal/controller/terminal-controller'
import { TerminalPane } from '../features/terminal/ui/terminal-pane'
import { FilePanel } from '../features/files/file-panel'
import { EditorPanel } from '../features/editor/editor-panel'
import { useTransfers } from '../features/transfers/use-transfers'
import { TransferDrawer } from '../features/transfers/transfer-drawer'
import { UploadDialog } from '../features/transfers/upload-dialog'
import { TunnelDrawer } from '../features/tunnels/tunnel-drawer'
import { SnippetPanel, type CommandTarget } from '../features/snippets/snippet-panel'
import { ImportDialog } from '../features/imports/import-dialog'
import { SettingsDialog, type SettingsSection } from '../features/settings/settings-dialog'
import { WelcomePage } from '../features/workspace/welcome-page'
import { TabStrip } from '../features/workspace/tab-strip'
import { PaneGrid } from '../features/workspace/pane-grid'
import {
  nextIndex,
  placeTab,
  removeTab,
  resizeSlots,
  type Slots,
  type Split,
} from '../features/workspace/layout'

interface Session {
  info: TerminalInfo
  controller: TerminalController
}
/** A tab is a host slot in the workspace; it may have no live session after a reload. */
interface Tab {
  key: string
  host: HostProfile
  session?: Session
}
type EditorTarget = { terminalID: string; path: string }
type Connecting = {
  host: HostProfile
  replaceKey?: string
  quickConnect?: QuickConnectTarget
  initialAttempt?: Attempt
  initialError?: string
  promptMode?: 'password' | 'passphrase'
}

const defaultPreferences: Preferences = {
  fontSize: 14,
  scrollback: 5000,
  cursorStyle: 'bar',
  keybindings: {},
}

const splitIcons: [Split, string, typeof Square][] = [
  ['single', '单屏', Square],
  ['columns', '左右分屏', Columns2],
  ['rows', '上下分屏', Rows2],
  ['grid', '四宫格', Grid2x2],
]

export function Workbench({
  auth,
  theme,
  toggleTheme,
  onLogout,
}: {
  auth: AuthState
  theme: 'dark' | 'light'
  toggleTheme: () => void
  onLogout: () => void
}) {
  const [tabs, setTabs] = useState<Tab[]>([])
  const [split, setSplit] = useState<Split>('single')
  const [slots, setSlots] = useState<Slots>([null])
  const [focusedSlot, setFocusedSlot] = useState(0)
  const [sizes, setSizes] = useState<number[]>([])
  const [preferences, setPreferences] = useState<Preferences>(defaultPreferences)
  const [restored, setRestored] = useState(false)
  const [query, setQuery] = useState('')
  const [sidebar, setSidebar] = useState(true)
  const [filesVisible, setFilesVisible] = useState(false)
  const [hostDialog, setHostDialog] = useState<HostProfile | 'new' | null>(null)
  const [connecting, setConnecting] = useState<Connecting | null>(null)
  const [quickConnecting, setQuickConnecting] = useState<string | null>(null)
  const [editor, setEditor] = useState<EditorTarget | null>(null)
  const [settings, setSettings] = useState<SettingsSection | null>(null)
  const [importing, setImporting] = useState(false)
  const [drawer, setDrawer] = useState<'transfers' | 'tunnels' | null>(null)
  const [commands, setCommands] = useState<'snippets' | 'broadcast' | null>(null)
  const [searchRequest, setSearchRequest] = useState(0)
  const [directories, setDirectories] = useState<Record<string, string>>({})
  const [cwd, setCwd] = useState<Record<string, { path: string; at: number }>>({})
  const [upload, setUpload] = useState<{
    tab: Tab
    items: UploadItem[]
    directory: string
    fromShell: boolean
  } | null>(null)
  const editorDirty = useRef(false)
  const directAttempt = useAttempt()
  const transfers = useTransfers()
  const notice = useNotice()
  const queries = useQueryClient()
  const hostsQuery = useQuery({ queryKey: ['hosts'], queryFn: api.hosts })
  const sshConfigQuery = useQuery({
    queryKey: ['sshConfigHosts'],
    queryFn: api.sshConfigHosts,
    enabled: !!auth.capabilities?.localFiles,
    staleTime: 30000,
  })
  const workspaceQuery = useQuery({
    queryKey: ['workspace'],
    queryFn: api.workspace,
    staleTime: Infinity,
  })
  const tunnelsQuery = useQuery({
    queryKey: ['tunnels'],
    queryFn: api.tunnels,
    refetchInterval: 15000,
  })
  const retained = useQuery({
    queryKey: ['retained'],
    queryFn: api.terminals,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })
  const hosts = useMemo(() => hostsQuery.data ?? [], [hostsQuery.data])
  const sshConfigHosts = useMemo(
    () => sshConfigQuery.data?.hosts ?? [],
    [sshConfigQuery.data],
  )
  const sshProfiles = useMemo(
    () => sshConfigHosts.map(sshConfigToProfile),
    [sshConfigHosts],
  )
  const localTerminalEnabled = !!auth.capabilities?.localTerminal
  const localProfile = useMemo(() => localHostProfile(), [])
  const currentBindings = useMemo(
    () => bindings(preferences.keybindings),
    [preferences.keybindings],
  )
  const bindingsRef = useRef(currentBindings)
  bindingsRef.current = currentBindings
  const focusedKey = slots[focusedSlot] ?? null
  const active = tabs.find((tab) => tab.key === focusedKey)
  const activeSession = active?.session
  const tabsRef = useRef(tabs)
  tabsRef.current = tabs
  const slotsRef = useRef(slots)
  slotsRef.current = slots

  const terminalOptions = useMemo(
    () => ({
      fontSize: preferences.fontSize,
      scrollback: preferences.scrollback,
      cursorStyle: preferences.cursorStyle,
    }),
    [preferences.fontSize, preferences.scrollback, preferences.cursorStyle],
  )

  useEffect(
    () => () => {
      for (const tab of tabsRef.current) tab.session?.controller.dispose()
    },
    [],
  )
  useEffect(() => {
    for (const tab of tabs) tab.session?.controller.setTheme(theme)
  }, [theme, tabs])
  useEffect(() => {
    for (const tab of tabs) tab.session?.controller.setOptions(terminalOptions)
  }, [terminalOptions, tabs])

  // Keep tab labels and markers in sync with edited host profiles.
  useEffect(() => {
    if (!hostsQuery.data) return
    const latest = new Map(hostsQuery.data.map((host) => [host.id, host]))
    setTabs((current) => {
      let changed = false
      const next = current.map((tab) => {
        const host = latest.get(tab.host.id)
        if (!host || host === tab.host) return tab
        changed = true
        return { ...tab, host }
      })
      return changed ? next : current
    })
  }, [hostsQuery.data])

  // Restore the saved layout once: tabs come back as disconnected
  // placeholders, never as pretend-live sessions.
  useEffect(() => {
    if (
      restored ||
      !workspaceQuery.data ||
      !hostsQuery.data ||
      (auth.capabilities?.localFiles && sshConfigQuery.isPending)
    )
      return
    const workspace = workspaceQuery.data
    setPreferences({ ...defaultPreferences, ...workspace.preferences })
    const known = new Map<string, HostProfile>([
      ...(localTerminalEnabled ? [[localProfile.id, localProfile] as const] : []),
      ...sshProfiles.map((host) => [host.id, host] as const),
      ...hostsQuery.data.map((host) => [host.id, host] as const),
    ])
    const placeholders = workspace.layout.tabs
      .map((tab) => known.get(tab.hostId))
      .filter((host): host is HostProfile => !!host)
      .map((host) => ({ key: crypto.randomUUID(), host }))
    const nextSplit = workspace.layout.split
    setTabs(placeholders)
    setSplit(nextSplit)
    setSizes(workspace.layout.sizes)
    setSlots(
      resizeSlots(
        [],
        nextSplit,
        placeholders.map((tab) => tab.key),
        null,
      ),
    )
    setFocusedSlot(0)
    setRestored(true)
  }, [
    restored,
    workspaceQuery.data,
    hostsQuery.data,
    sshConfigQuery.isPending,
    sshProfiles,
    auth.capabilities?.localFiles,
    localTerminalEnabled,
    localProfile,
  ])
  useEffect(() => {
    if (workspaceQuery.isError && !restored) setRestored(true)
  }, [workspaceQuery.isError, restored])

  // Persist the arrangement (host references only) after changes settle.
  useEffect(() => {
    if (!restored) return
    const timer = window.setTimeout(() => {
      const workspace: Workspace = {
        layout: {
          split,
          tabs: tabs
            .filter((tab) => !tab.host.id.startsWith('quick:'))
            .slice(0, 8)
            .map((tab) => ({ hostId: tab.host.id })),
          sizes,
        },
        preferences,
      }
      void api
        .saveWorkspace(workspace)
        .then((saved) => queries.setQueryData(['workspace'], saved))
        .catch(() => undefined)
    }, 800)
    return () => clearTimeout(timer)
  }, [restored, split, tabs, sizes, preferences, queries])

  const setDirty = useCallback((value: boolean) => {
    editorDirty.current = value
  }, [])
  function canCloseEditor() {
    return !editorDirty.current || window.confirm('编辑器有未保存的修改，确认放弃？')
  }
  function clearEditor() {
    editorDirty.current = false
    setEditor(null)
  }
  function focusTab(key: string) {
    if (editor && !canCloseEditor()) return
    clearEditor()
    const placed = placeTab(slots, focusedSlot, key)
    setSlots(placed.slots)
    setFocusedSlot(placed.focusedSlot)
  }
  function changeSplit(next: Split) {
    setSplit(next)
    const resized = resizeSlots(
      slots,
      next,
      tabs.map((tab) => tab.key),
      focusedKey,
    )
    setSlots(resized)
    setFocusedSlot(Math.max(0, resized.indexOf(focusedKey)))
    setSizes([])
  }

  const isShortcut = useCallback(
    (event: KeyboardEvent) => actionFor(event, bindingsRef.current) !== undefined,
    [],
  )

  function sessionFor(info: TerminalInfo, host: HostProfile, key: string): Session {
    const controller = new TerminalController(
      info.id,
      host.hostname,
      {
        directory: (path) =>
          setCwd((previous) => ({ ...previous, [info.id]: { path, at: Date.now() } })),
        notice,
        shortcut: isShortcut,
        focused: () => {
          const slot = slotsRef.current.indexOf(key)
          if (slot >= 0) setFocusedSlot(slot)
        },
      },
      theme,
      terminalOptions,
    )
    return { info, controller }
  }

  function completeConnection(host: HostProfile, replaceKey: string | undefined, result: LaunchResult) {
    if (!result.terminal) return
    const key = replaceKey ?? crypto.randomUUID()
    const session = sessionFor(result.terminal, host, key)
    const previous = tabsRef.current.find((tab) => tab.key === replaceKey)?.session
    if (previous) {
      previous.controller.dispose()
      void api.closeTerminal(previous.info.id).catch(() => undefined)
    }
    setTabs((current) =>
      replaceKey && current.some((tab) => tab.key === replaceKey)
        ? current.map((tab) => (tab.key === replaceKey ? { ...tab, host, session } : tab))
        : [...current, { key, host, session }],
    )
    if (!editor) {
      const placed = placeTab(slotsRef.current, focusedSlot, key)
      setSlots(placed.slots)
      setFocusedSlot(placed.focusedSlot)
    }
    setConnecting(null)
    void queries.invalidateQueries({ queryKey: ['hosts'] })
  }

  function connected(result: LaunchResult) {
    if (!connecting || !result.terminal) return
    completeConnection(connecting.host, connecting.replaceKey, result)
  }

  async function startDirectConnect(
    host: HostProfile,
    options?: { replaceKey?: string; quickConnect?: QuickConnectTarget },
  ) {
    const requiresExplicitInput =
      !isLocalHost(host) &&
      ((host.authMode === 'password' && !host.credentialId) ||
        (host.authMode === 'key' && !host.identityFile && !host.credentialId))
    if (requiresExplicitInput) {
      setConnecting({ host, replaceKey: options?.replaceKey, quickConnect: options?.quickConnect })
      return
    }
    setQuickConnecting(host.name)
    try {
      const final = await directAttempt.start({
        purpose: 'terminal',
        hostId: options?.quickConnect ? undefined : host.id,
        quickConnect: options?.quickConnect,
        rows: 28,
        cols: 100,
        credentials: {},
        useSaved: !!host.credentialId,
      })
      setQuickConnecting(null)
      if (final.state === 'connected' && final.result) {
        if (final.result.warning) notice(final.result.warning)
        completeConnection(host, options?.replaceKey, final.result)
        return
      }
      if (final.state === 'prompt') {
        setConnecting({
          host,
          replaceKey: options?.replaceKey,
          quickConnect: options?.quickConnect,
          initialAttempt: final,
        })
        return
      }
      if (final.state === 'failed') {
        const code = final.error?.code
        setConnecting({
          host,
          replaceKey: options?.replaceKey,
          quickConnect: options?.quickConnect,
          promptMode:
            code === 'passphrase_required'
              ? 'passphrase'
              : code === 'password_required'
                ? 'password'
                : undefined,
          initialError: final.error?.message ?? '连接失败。',
        })
      }
    } catch (problem) {
      setQuickConnecting(null)
      if (problem instanceof DOMException && problem.name === 'AbortError') return
      setConnecting({
        host,
        replaceKey: options?.replaceKey,
        quickConnect: options?.quickConnect,
        initialError: messageOf(problem),
      })
    }
  }

  function handleQuickConnect(target: QuickConnectTarget) {
    const profile = quickTargetToProfile(target)
    void startDirectConnect(profile, { quickConnect: target })
  }

  function openLocalTerminal() {
    void startDirectConnect(localProfile)
  }

  async function closeTab(key: string) {
    const tab = tabs.find((candidate) => candidate.key === key)
    if (!tab) return
    if (tab.session && transfers.pending(tab.session.info.id)) {
      notice('此会话仍有上传任务，请等待完成或在任务抽屉中取消后关闭。')
      return
    }
    if (tab.session && editor?.terminalID === tab.session.info.id && !canCloseEditor()) return
    if (tab.session) {
      try {
        await api.closeTerminal(tab.session.info.id)
      } catch (problem) {
        notice(messageOf(problem))
      }
      tab.session.controller.dispose()
      if (editor?.terminalID === tab.session.info.id) clearEditor()
    }
    const order = tabs.map((candidate) => candidate.key)
    setTabs((current) => current.filter((candidate) => candidate.key !== key))
    setSlots(removeTab(slots, key, order))
  }

  function reorder(from: string, to: string) {
    setTabs((current) => {
      const next = [...current]
      const source = next.findIndex((tab) => tab.key === from)
      const target = next.findIndex((tab) => tab.key === to)
      if (source < 0 || target < 0) return current
      const [moved] = next.splice(source, 1)
      next.splice(target, 0, moved)
      return next
    })
  }

  function prepareUpload(tab: Tab, items: UploadItem[], directory?: string) {
    if (!items.length || !tab.session) return
    const report = cwd[tab.session.info.id]
    const fromShell = !directory && !!report && Date.now() - report.at < 5 * 60 * 1000
    setUpload({
      tab,
      items,
      directory: directory ?? (fromShell ? report.path : (directories[tab.session.info.id] ?? '')),
      fromShell,
    })
  }

  function updatePreferences(next: Preferences) {
    setPreferences({
      ...next,
      fontSize: Math.min(28, Math.max(10, next.fontSize)),
      scrollback: Math.min(50000, Math.max(500, next.scrollback)),
    })
  }

  function runAction(action: ActionID) {
    {
      const order = tabs.map((tab) => tab.key)
      switch (action) {
        case 'hosts.search':
          setSidebar(true)
          requestAnimationFrame(() => document.getElementById('host-search')?.focus())
          break
        case 'connect.quick':
          setHostDialog('new')
          break
        case 'tab.next':
        case 'tab.previous': {
          const index = nextIndex(
            order.length,
            order.indexOf(focusedKey ?? ''),
            action === 'tab.next' ? 1 : -1,
          )
          if (index >= 0) focusTab(order[index])
          break
        }
        case 'tab.close':
          if (focusedKey) void closeTab(focusedKey)
          break
        case 'pane.next':
          setFocusedSlot((current) => nextIndex(slots.length, current, 1))
          break
        case 'layout.single':
        case 'layout.columns':
        case 'layout.rows':
        case 'layout.grid':
          changeSplit(action.slice('layout.'.length) as Split)
          break
        case 'files.toggle':
          setFilesVisible((visible) => !visible)
          break
        case 'transfers.toggle':
          setDrawer((current) => (current === 'transfers' ? null : 'transfers'))
          break
        case 'tunnels.toggle':
          setDrawer((current) => (current === 'tunnels' ? null : 'tunnels'))
          break
        case 'terminal.search':
          setSearchRequest((count) => count + 1)
          break
        case 'snippets.open':
          setCommands('snippets')
          break
        case 'broadcast.open':
          setCommands('broadcast')
          break
        case 'font.larger':
        case 'font.smaller':
          setPreferences((current) => ({
            ...current,
            fontSize: Math.min(
              28,
              Math.max(10, current.fontSize + (action === 'font.larger' ? 1 : -1)),
            ),
          }))
          break
      }
    }
  }
  const runActionRef = useRef(runAction)
  runActionRef.current = runAction

  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      const target = event.target instanceof Element ? event.target : null
      const inTerminal = !!target?.closest('.xterm')
      const editing =
        !inTerminal &&
        !!target?.closest('input, textarea, select, [contenteditable], .monaco-editor')
      if (editing || document.querySelector('dialog[open]')) return
      const action = actionFor(event, bindingsRef.current)
      if (!action) return
      event.preventDefault()
      runActionRef.current(action)
    }
    window.addEventListener('keydown', listener)
    return () => window.removeEventListener('keydown', listener)
  }, [])

  async function closeRetained() {
    const live = new Set(tabs.map((tab) => tab.session?.info.id))
    await Promise.all(
      (retained.data ?? [])
        .filter((session) => !live.has(session.id))
        .map((session) => api.closeTerminal(session.id).catch(() => undefined)),
    )
    await queries.invalidateQueries({ queryKey: ['retained'] })
  }
  const live = new Set(tabs.map((tab) => tab.session?.info.id))
  const oldSessions = (retained.data ?? []).filter((session) => !live.has(session.id))
  const pendingCount = transfers.transfers.filter((transfer) =>
    ['queued', 'uploading'].includes(transfer.status),
  ).length
  const runningTunnels = (tunnelsQuery.data ?? []).filter(
    (tunnel) => tunnel.status.state === 'running',
  ).length
  const allKnownHosts = useMemo(() => [...sshProfiles, ...hosts], [sshProfiles, hosts])
  const hostByID = useMemo(
    () => new Map(allKnownHosts.map((host) => [host.id, host])),
    [allKnownHosts],
  )
  const commandTargets: CommandTarget[] = tabs.flatMap((tab) =>
    tab.session
      ? [
          {
            id: tab.session.info.id,
            name: tab.host.name,
            address: isLocalHost(tab.host)
              ? 'localhost (本机)'
              : `${tab.host.username}@${tab.host.hostname}:${tab.host.port}`,
            production: !!tab.host.production,
            ready: () => tab.session?.controller.ready() ?? false,
            send: (text: string) => tab.session?.controller.sendText(text) ?? false,
          },
        ]
      : [],
  )
  const activeTarget = commandTargets.find((target) => target.id === activeSession?.info.id)

  function renderSlot(slot: number) {
    const key = slots[slot]
    const tab = tabs.find((candidate) => candidate.key === key)
    const focused = slot === focusedSlot
    if (!tab)
      return (
        <div
          className={`pane-empty ${focused ? 'focused' : ''}`}
          onMouseDown={() => setFocusedSlot(slot)}
        >
          <Terminal size={22} />
          <p>空白分屏</p>
          <span>点击上方标签放入此处，或从左侧连接主机。</span>
        </div>
      )
    if (!tab.session)
      return (
        <div
          className={`pane-empty placeholder ${focused ? 'focused' : ''}`}
          onMouseDown={() => setFocusedSlot(slot)}
        >
          {isLocalHost(tab.host) ? <Terminal size={22} /> : <Server size={22} />}
          <p>{tab.host.name} · 未连接</p>
          <span>页面刷新不会恢复原有 Shell 画面，点击下方按钮新建连接。</span>
          <button
            className="button secondary small"
            onClick={() => void startDirectConnect(tab.host, { replaceKey: tab.key })}
          >
            <RotateCcw size={14} />
            重新连接
          </button>
        </div>
      )
    const session = tab.session
    const localTab = isLocalHost(tab.host)
    return (
      <TerminalPane
        key={session.info.id}
        controller={session.controller}
        title={tab.host.name}
        protocol={localTab ? 'LOCAL' : 'SSH'}
        production={!!tab.host.production}
        focused={focused && !editor}
        searchRequest={focused ? searchRequest : 0}
        onFocus={() => setFocusedSlot(slot)}
        onDrop={localTab ? undefined : (items) => prepareUpload(tab, items)}
        onReconnect={() => void startDirectConnect(tab.host, { replaceKey: tab.key })}
      />
    )
  }

  const connectingJump = connecting?.host.jumpHostId
    ? hostByID.get(connecting.host.jumpHostId)
    : undefined

  return (
    <div className="workbench">
      <div className="workbench-body">
        {sidebar && (
          <HostSidebar
            hosts={hosts}
            sshConfigHosts={sshConfigHosts}
            sshConfigLoading={sshConfigQuery.isFetching}
            query={query}
            onQuery={setQuery}
            activeHost={active?.host.id}
            connectedHosts={new Set(tabs.filter((tab) => tab.session).map((tab) => tab.host.id))}
            onConnect={(host) => void startDirectConnect(host)}
            onEdit={setHostDialog}
            onCreate={() => setHostDialog('new')}
            onImport={() => setImporting(true)}
            onRefreshSSHConfig={() => void sshConfigQuery.refetch()}
            localTerminal={localTerminalEnabled}
            onLocalTerminal={openLocalTerminal}
            searchShortcut={
              currentBindings['hosts.search'] ? describe(currentBindings['hosts.search']) : ''
            }
          />
        )}
        <main className="main-workspace">
          <div className="tab-bar">
            <button
              className="icon-button sidebar-toggle"
              aria-label={sidebar ? '收起主机列表' : '展开主机列表'}
              title={sidebar ? '收起主机列表' : '展开主机列表'}
              onClick={() => setSidebar(!sidebar)}
            >
              {sidebar ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
            </button>
            {tabs.length === 0 ? (
              <div className="overview-tab">
                <Terminal size={14} />
                <span>快速启动</span>
              </div>
            ) : (
              <TabStrip
                tabs={tabs.map((tab) => ({
                  key: tab.key,
                  title: tab.host.name,
                  color: tab.host.color,
                  production: !!tab.host.production,
                  connected: !!tab.session,
                  visible: slots.includes(tab.key),
                  focused: tab.key === focusedKey,
                }))}
                onSelect={focusTab}
                onClose={(key) => void closeTab(key)}
                onReorder={reorder}
              />
            )}
            <button
              className="icon-button new-session"
              title="新建主机配置"
              aria-label="新建连接配置"
              onClick={() => setHostDialog('new')}
            >
              <Plus size={15} />
            </button>
            {localTerminalEnabled && (
              <button
                className="icon-button new-session"
                title="打开本地终端"
                aria-label="打开本地终端"
                onClick={openLocalTerminal}
              >
                <Terminal size={15} />
              </button>
            )}
            <div className="tab-spacer" />
            {quickConnecting && (
              <div className="connecting-pill" role="status">
                <LoaderCircle size={13} className="spin" />
                <span>正在连接 {quickConnecting}…</span>
                <button
                  type="button"
                  aria-label="取消连接"
                  onClick={() => {
                    directAttempt.cancel()
                    setQuickConnecting(null)
                  }}
                >
                  <X size={12} />
                </button>
              </div>
            )}
            <div className="topbar-tools">
              {tabs.length > 0 && (
                <div className="split-switcher" role="group" aria-label="分屏布局">
                  {splitIcons.map(([value, label, Icon]) => (
                    <button
                      key={value}
                      className={`icon-button ${split === value ? 'selected' : ''}`}
                      aria-label={label}
                      aria-pressed={split === value}
                      title={label}
                      onClick={() => changeSplit(value)}
                    >
                      <Icon size={14} />
                    </button>
                  ))}
                  <button
                    className="icon-button"
                    aria-label="多主机广播"
                    title="多主机广播"
                    disabled={commandTargets.length === 0}
                    onClick={() => setCommands('broadcast')}
                  >
                    <Megaphone size={14} />
                  </button>
                </div>
              )}
              {active && activeSession && !isLocalHost(active.host) && (
                <button
                  className={`topbar-chip ${filesVisible ? 'active' : ''}`}
                  aria-label="切换文件面板"
                  aria-pressed={filesVisible}
                  title="SFTP 文件管理"
                  onClick={() => setFilesVisible(!filesVisible)}
                >
                  <Files size={14} />
                  <span>文件</span>
                </button>
              )}
              <button
                className={`icon-button tool-badge-btn ${drawer === 'transfers' ? 'active' : ''}`}
                title="传输任务"
                aria-label="显示传输任务"
                onClick={() => setDrawer(drawer === 'transfers' ? null : 'transfers')}
              >
                <ArrowDownToLine size={15} />
                {pendingCount > 0 && <i>{pendingCount}</i>}
              </button>
              <button
                className={`icon-button tool-badge-btn ${drawer === 'tunnels' ? 'active' : ''}`}
                title="端口转发隧道"
                aria-label="显示端口转发隧道"
                onClick={() => setDrawer(drawer === 'tunnels' ? null : 'tunnels')}
              >
                <Network size={15} />
                {runningTunnels > 0 && <i>{runningTunnels}</i>}
              </button>
              <button
                className={`icon-button ${commands ? 'active' : ''}`}
                title="命令片段"
                aria-label="命令片段与广播"
                onClick={() => setCommands('snippets')}
              >
                <Command size={15} />
              </button>
              <button
                className="icon-button"
                title={theme === 'dark' ? '切换浅色主题' : '切换深色主题'}
                aria-label={theme === 'dark' ? '切换浅色主题' : '切换深色主题'}
                onClick={toggleTheme}
              >
                {theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />}
              </button>
              <button
                className="icon-button"
                title="工作区设置"
                aria-label="工作区设置"
                onClick={() => setSettings('appearance')}
              >
                <Settings2 size={15} />
              </button>
            </div>
          </div>
          {hostsQuery.isError && (
            <div className="workspace-warning">
              {messageOf(hostsQuery.error)}
              <button onClick={() => void hostsQuery.refetch()}>重试</button>
            </div>
          )}
          {oldSessions.length > 0 && (
            <div className="workspace-warning">
              检测到 {oldSessions.length} 个旧会话，未连接的会话会在保留期后自动关闭。
              <button onClick={() => void closeRetained()}>立即清理</button>
            </div>
          )}
          {tabs.length === 0 ? (
            <WelcomePage
              hosts={hosts}
              sshConfigHosts={sshConfigHosts}
              version={auth.version}
              localTerminal={localTerminalEnabled}
              onLocalTerminal={openLocalTerminal}
              onCreate={() => setHostDialog('new')}
              onImport={() => setImporting(true)}
              onTunnels={() => setDrawer('tunnels')}
              onConnect={(host) => void startDirectConnect(host)}
              onQuickConnect={handleQuickConnect}
            />
          ) : (
            <div className="session-workspace">
              <div className="terminal-stage" style={{ display: editor ? 'none' : undefined }}>
                <PaneGrid split={split} sizes={sizes} onSizes={setSizes} render={renderSlot} />
              </div>
              {editor && (
                <EditorPanel
                  key={`${editor.terminalID}:${editor.path}`}
                  source={{ kind: 'remote', ...editor }}
                  theme={theme}
                  onClose={clearEditor}
                  onDirtyChange={setDirty}
                />
              )}
              {active && activeSession && !isLocalHost(active.host) && filesVisible && (
                <FilePanel
                  key={activeSession.info.id}
                  terminalID={activeSession.info.id}
                  hostLabel={`${active.host.username}@${active.host.hostname}:${active.host.port}`}
                  initialPath={directories[activeSession.info.id] ?? '.'}
                  cwd={cwd[activeSession.info.id]?.path}
                  onDirectory={(path) =>
                    setDirectories((previous) =>
                      previous[activeSession.info.id] === path
                        ? previous
                        : { ...previous, [activeSession.info.id]: path },
                    )
                  }
                  onUpload={(items, directory) => prepareUpload(active, items, directory)}
                  onEdit={(path) => {
                    if (editor && !canCloseEditor()) return
                    editorDirty.current = false
                    setEditor({ terminalID: activeSession.info.id, path })
                  }}
                  onHide={() => setFilesVisible(false)}
                />
              )}
            </div>
          )}
          {drawer === 'transfers' && (
            <TransferDrawer
              transfers={transfers.transfers}
              onCancel={transfers.cancel}
              onRetry={transfers.retry}
              onClear={transfers.clearFinished}
              onClose={() => setDrawer(null)}
            />
          )}
          {drawer === 'tunnels' && (
            <TunnelDrawer auth={auth} hosts={allKnownHosts} onClose={() => setDrawer(null)} />
          )}
        </main>
      </div>
      {hostDialog && (
        <HostDialog
          auth={auth}
          hosts={hosts}
          host={hostDialog === 'new' ? undefined : hostDialog}
          onClose={() => setHostDialog(null)}
        />
      )}
      {connecting && (
        <ConnectionDialog
          auth={auth}
          host={connecting.host}
          jumpHost={connectingJump}
          quickConnect={connecting.quickConnect}
          initialAttempt={connecting.initialAttempt}
          initialError={connecting.initialError}
          promptMode={connecting.promptMode}
          purpose={{ kind: 'terminal', rows: 28, cols: 100 }}
          onClose={() => setConnecting(null)}
          onConnected={connected}
        />
      )}
      {upload && upload.tab.session && (
        <UploadDialog
          host={`${upload.tab.host.username}@${upload.tab.host.hostname}:${upload.tab.host.port}`}
          items={upload.items}
          suggestedDirectory={upload.directory}
          fromShell={upload.fromShell}
          onClose={() => setUpload(null)}
          onConfirm={(directory) => {
            try {
              transfers.enqueue(
                upload.tab.session!.info.id,
                upload.tab.host.name,
                directory,
                upload.items,
              )
              setDrawer('transfers')
              setUpload(null)
            } catch (problem) {
              notice(messageOf(problem))
            }
          }}
        />
      )}
      {commands && (
        <SnippetPanel
          mode={commands}
          active={activeTarget}
          targets={commandTargets}
          onModeChange={setCommands}
          onClose={() => {
            setCommands(null)
            activeSession?.controller.focus()
          }}
        />
      )}
      {importing && (
        <ImportDialog
          auth={auth}
          onClose={() => setImporting(false)}
          onImported={() => void queries.invalidateQueries({ queryKey: ['hosts'] })}
        />
      )}
      {settings && (
        <SettingsDialog
          auth={auth}
          theme={theme}
          toggleTheme={toggleTheme}
          preferences={preferences}
          onPreferences={updatePreferences}
          initialSection={settings}
          canCloseEditor={canCloseEditor}
          onLogout={onLogout}
          onClose={() => setSettings(null)}
        />
      )}
    </div>
  )
}
