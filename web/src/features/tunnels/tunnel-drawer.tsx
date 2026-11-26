import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  CircleAlert,
  CircleStop,
  LoaderCircle,
  MapPin,
  Pencil,
  Play,
  Plus,
  Square,
  Trash2,
  Waypoints,
  X,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { AuthState, HostProfile, LaunchResult, Tunnel } from '../../shared/api/types'
import { formatBytes } from '../../shared/lib/format'
import { useNotice } from '../../shared/ui/notices'
import { ConnectionDialog } from '../connect/connection-dialog'
import { TunnelDialog } from './tunnel-dialog'
import { endpoint, listenLocationNote } from './tunnel-form'
import './tunnels.css'

const stateLabel = { running: '运行中', stopped: '已停止', failed: '失败' } as const

interface Launch {
  tunnel: Tunnel
  host: HostProfile
  jumpHost?: HostProfile
}

function resolveHosts(tunnel: Tunnel, hosts: readonly HostProfile[]) {
  const host = hosts.find((item) => item.id === tunnel.hostId)
  const jumpHost = host?.jumpHostId ? hosts.find((item) => item.id === host.jumpHostId) : undefined
  const problem = !host
    ? '找不到此隧道使用的 SSH 主机，请编辑隧道重新选择。'
    : host.jumpHostId && !jumpHost
      ? `找不到 ${host.name} 配置的跳板机，请先修正主机设置。`
      : undefined
  return { host, jumpHost, problem }
}

export function TunnelDrawer({
  auth,
  hosts,
  onClose,
}: {
  auth: AuthState
  hosts: HostProfile[]
  onClose: () => void
}) {
  const tunnels = useQuery({ queryKey: ['tunnels'], queryFn: api.tunnels, refetchInterval: 3000 })
  const queries = useQueryClient()
  const notice = useNotice()
  const [editing, setEditing] = useState<Tunnel | 'new' | null>(null)
  const [launch, setLaunch] = useState<Launch | null>(null)
  const [pending, setPending] = useState<ReadonlySet<string>>(new Set())
  const inflight = useRef(new Set<string>())
  const list = tunnels.data ?? []
  const side = auth.remote ? '服务器' : '本地'

  // The ref blocks a second request from a fast double click before React re-renders.
  async function act(tunnel: Tunnel, action: () => Promise<void>) {
    if (inflight.current.has(tunnel.id)) return
    inflight.current.add(tunnel.id)
    setPending(new Set(inflight.current))
    try {
      await action()
    } catch (problem) {
      notice(`${tunnel.name}：${messageOf(problem)}`)
    } finally {
      inflight.current.delete(tunnel.id)
      setPending(new Set(inflight.current))
      void queries.invalidateQueries({ queryKey: ['tunnels'] })
    }
  }

  function stop(tunnel: Tunnel) {
    const active = tunnel.status.activeConnections
    if (
      active > 0 &&
      !window.confirm(`隧道「${tunnel.name}」还有 ${active} 个活动连接，停止会立即断开它们。继续？`)
    )
      return
    void act(tunnel, () => api.stopTunnel(tunnel.id))
  }

  function remove(tunnel: Tunnel) {
    if (
      !window.confirm(
        `删除隧道「${tunnel.name}」（${endpoint(tunnel.listenHost, tunnel.listenPort)} → ${endpoint(tunnel.targetHost, tunnel.targetPort)}）？此操作不可撤销。`,
      )
    )
      return
    void act(tunnel, () => api.deleteTunnel(tunnel.id))
  }

  function started(target: Launch, result: LaunchResult) {
    setLaunch(null)
    void queries.invalidateQueries({ queryKey: ['tunnels'] })
    const listen =
      result.tunnel?.status.listen ?? endpoint(target.tunnel.listenHost, target.tunnel.listenPort)
    notice(`隧道「${target.tunnel.name}」已启动，监听 ${listen}。`)
  }

  return (
    <section className="transfer-drawer tunnel-drawer" aria-label="端口转发隧道">
      <header className="panel-heading">
        <div>
          <Waypoints size={16} />
          <h2>端口转发隧道</h2>
          <span className="subtle-badge">{list.length}</span>
        </div>
        <div>
          <button
            className="text-button tunnel-new"
            disabled={hosts.length === 0}
            title={hosts.length === 0 ? '请先添加 SSH 主机' : undefined}
            onClick={() => setEditing('new')}
          >
            <Plus size={14} />
            新建隧道
          </button>
          <button className="icon-button" aria-label="收起端口转发隧道" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
      </header>
      <p className="tunnel-location">
        <MapPin size={13} />
        {listenLocationNote(auth.remote)}
      </p>
      <div className="transfer-list">
        {tunnels.isPending ? (
          <div className="transfer-empty" role="status">
            <LoaderCircle size={15} className="spin" />
            正在读取隧道…
          </div>
        ) : tunnels.isError && list.length === 0 ? (
          <div className="transfer-empty" role="alert">
            {messageOf(tunnels.error)}
            <button className="text-button" onClick={() => void tunnels.refetch()}>
              重试
            </button>
          </div>
        ) : list.length === 0 ? (
          <div className="transfer-empty">
            暂无隧道。本地转发（ssh -L）把服务所在机器回环地址上的一个端口，经 SSH
            主机转发到从该主机可达的目标地址。
          </div>
        ) : (
          list.map((tunnel) => {
            const { host, jumpHost, problem } = resolveHosts(tunnel, hosts)
            const { status } = tunnel
            const running = status.state === 'running'
            const busy = pending.has(tunnel.id)
            const via = host
              ? `${host.name}${jumpHost ? `（跳板 ${jumpHost.name}）` : ''}`
              : '未知主机'
            const route = `${side} ${endpoint(tunnel.listenHost, tunnel.listenPort)} → 经 ${via} → 目标 ${endpoint(tunnel.targetHost, tunnel.targetPort)}`
            return (
              <div className="transfer-row tunnel-row" key={tunnel.id}>
                <div
                  className={`transfer-state ${running ? 'done' : status.state === 'failed' ? 'error' : ''}`}
                >
                  {running ? (
                    <Activity size={17} />
                  ) : status.state === 'failed' ? (
                    <CircleAlert size={17} />
                  ) : (
                    <CircleStop size={17} />
                  )}
                </div>
                <div className="transfer-description">
                  <strong>{tunnel.name}</strong>
                  <small title={route}>{route}</small>
                  {running ? (
                    <small className="tunnel-stats">
                      连接 {status.activeConnections} 活动 / {status.totalConnections} 累计 · 发送{' '}
                      {formatBytes(status.bytesSent)} · 接收 {formatBytes(status.bytesReceived)}
                    </small>
                  ) : status.state === 'failed' && status.error ? (
                    <small className="tunnel-error" title={status.error.message}>
                      {status.error.message}
                    </small>
                  ) : null}
                </div>
                <span className={`transfer-label tunnel-state-label ${status.state}`}>
                  {stateLabel[status.state]}
                </span>
                <div className="tunnel-actions">
                  {running ? (
                    <button
                      className="button small secondary"
                      disabled={busy}
                      aria-label={`停止隧道 ${tunnel.name}`}
                      onClick={() => stop(tunnel)}
                    >
                      {busy ? <LoaderCircle size={13} className="spin" /> : <Square size={13} />}
                      停止
                    </button>
                  ) : (
                    <button
                      className="button small primary"
                      disabled={busy || !host || !!problem}
                      title={problem}
                      aria-label={`启动隧道 ${tunnel.name}`}
                      onClick={() => {
                        if (host) setLaunch({ tunnel, host, jumpHost })
                      }}
                    >
                      <Play size={13} />
                      启动
                    </button>
                  )}
                  <button
                    className="icon-button"
                    disabled={running || busy}
                    title={running ? '运行中的隧道不能修改，请先停止' : '编辑隧道'}
                    aria-label={`编辑隧道 ${tunnel.name}`}
                    onClick={() => setEditing(tunnel)}
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    className="icon-button danger-hover"
                    disabled={running || busy}
                    title={running ? '运行中的隧道不能删除，请先停止' : '删除隧道'}
                    aria-label={`删除隧道 ${tunnel.name}`}
                    onClick={() => remove(tunnel)}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            )
          })
        )}
      </div>
      {editing && (
        <TunnelDialog
          remote={auth.remote}
          hosts={hosts}
          tunnel={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
        />
      )}
      {launch && (
        <ConnectionDialog
          auth={auth}
          host={launch.host}
          jumpHost={launch.jumpHost}
          purpose={{ kind: 'tunnel', tunnelID: launch.tunnel.id, label: launch.tunnel.name }}
          onClose={() => setLaunch(null)}
          onConnected={(result) => started(launch, result)}
        />
      )}
    </section>
  )
}
