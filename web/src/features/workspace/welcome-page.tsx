import { useMemo, useState, type FormEvent } from 'react'
import { ArrowRight, FileInput, Network, Plus, Server, Terminal } from 'lucide-react'
import type { HostProfile, QuickConnectTarget, SSHConfigHost } from '../../shared/api/types'
import { parseQuickTarget, sshConfigToProfile } from '../hosts/host-helpers'

export function WelcomePage({
  hosts,
  sshConfigHosts,
  version,
  localTerminal,
  onLocalTerminal,
  onCreate,
  onImport,
  onTunnels,
  onConnect,
  onQuickConnect,
}: {
  hosts: HostProfile[]
  sshConfigHosts: SSHConfigHost[]
  version: string
  localTerminal?: boolean
  onLocalTerminal?: () => void
  onCreate: () => void
  onImport: () => void
  onTunnels: () => void
  onConnect: (host: HostProfile) => void
  onQuickConnect: (target: QuickConnectTarget) => void
}) {
  const [address, setAddress] = useState('')
  const [error, setError] = useState('')

  const sshProfiles = useMemo(
    () => sshConfigHosts.map(sshConfigToProfile),
    [sshConfigHosts],
  )

  const recentHosts = useMemo(
    () =>
      [...hosts]
        .filter((host) => host.lastUsedAt)
        .sort((a, b) => (b.lastUsedAt ?? '').localeCompare(a.lastUsedAt ?? ''))
        .slice(0, 6),
    [hosts],
  )

  const matchingHosts = useMemo(() => {
    const q = address.trim().toLocaleLowerCase()
    const combined = [...hosts, ...sshProfiles]
    if (!q) return []
    return combined
      .filter((host) =>
        `${host.name} ${host.hostname} ${host.username}`.toLocaleLowerCase().includes(q),
      )
      .slice(0, 6)
  }, [address, hosts, sshProfiles])

  function submit(event: FormEvent) {
    event.preventDefault()
    const trimmed = address.trim()
    if (!trimmed) {
      setError('请输入主机地址（例如 user@192.168.1.10）或 ~/.ssh/config 别名')
      return
    }
    setError('')
    const exact = [...sshProfiles, ...hosts].find(
      (host) => host.name.toLocaleLowerCase() === trimmed.toLocaleLowerCase(),
    )
    if (exact) {
      onConnect(exact)
      return
    }
    const parsed = parseQuickTarget(trimmed)
    if (!parsed) {
      setError('格式无法识别，请输入 host 或 user@host[:port]')
      return
    }
    onQuickConnect(parsed)
  }

  return (
    <div className="welcome-page">
      <div className="launchpad">
        <div className="launchpad-head">
          <div>
            <h1>快速连接</h1>
            <p>输入地址回车直连，或点击下方已保存主机与本机 ~/.ssh/config 服务器</p>
          </div>
          <div className="launchpad-actions">
            {localTerminal && onLocalTerminal && (
              <button className="button primary small" onClick={onLocalTerminal}>
                <Terminal size={14} />
                本地终端
              </button>
            )}
            <button className="button secondary small" onClick={onCreate}>
              <Plus size={14} />
              新建主机
            </button>
            <button className="button secondary small" onClick={onImport}>
              <FileInput size={14} />
              导入配置
            </button>
            <button className="button secondary small" onClick={onTunnels}>
              <Network size={14} />
              端口转发
            </button>
          </div>
        </div>

        <form className="quick-bar" onSubmit={submit}>
          <Terminal size={16} />
          <input
            aria-label="快速连接地址"
            placeholder="ssh user@hostname[:port] 或输入 ~/.ssh/config 别名…"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            value={address}
            onChange={(event) => {
              setAddress(event.target.value)
              if (error) setError('')
            }}
          />
          <button type="submit" className="button primary small">
            连接
            <ArrowRight size={14} />
          </button>
        </form>
        {error && (
          <div className="inline-error" role="alert">
            {error}
          </div>
        )}

        {matchingHosts.length > 0 && (
          <section className="launchpad-section">
            <h2>匹配主机</h2>
            <HostGrid hosts={matchingHosts} onConnect={onConnect} />
          </section>
        )}

        {recentHosts.length > 0 && !address.trim() && (
          <section className="launchpad-section">
            <h2>
              最近连接 <span>{recentHosts.length}</span>
            </h2>
            <HostGrid hosts={recentHosts} onConnect={onConnect} />
          </section>
        )}

        {sshProfiles.length > 0 && !address.trim() && (
          <section className="launchpad-section">
            <h2>
              Mac SSH Config (~/.ssh/config) <span>{sshProfiles.length} 台</span>
            </h2>
            <HostGrid hosts={sshProfiles.slice(0, 12)} onConnect={onConnect} badge="CONFIG" />
          </section>
        )}

        {hosts.length > 0 && !address.trim() && (
          <section className="launchpad-section">
            <h2>
              已保存主机 <span>{hosts.length} 台</span>
            </h2>
            <HostGrid hosts={hosts.slice(0, 8)} onConnect={onConnect} />
          </section>
        )}

        {hosts.length === 0 && sshProfiles.length === 0 && !address.trim() && (
          <div className="launchpad-empty">
            <Server size={28} strokeWidth={1.3} />
            <p>暂无已配置服务器</p>
            <span>
              在上方输入 <code>user@host</code> 直接连接，或点击“新建主机”保存常用服务器。
            </span>
          </div>
        )}
      </div>
      <div className="welcome-corner">SSH Workbench · {version}</div>
    </div>
  )
}

function HostGrid({
  hosts,
  onConnect,
  badge,
}: {
  hosts: HostProfile[]
  onConnect: (host: HostProfile) => void
  badge?: string
}) {
  return (
    <div className="quick-hosts">
      {hosts.map((host) => (
        <button
          key={host.id}
          className={`quick-host host-${host.color}`}
          onClick={() => onConnect(host)}
        >
          <span className="host-symbol">
            <Server size={15} />
          </span>
          <span className="quick-host-meta">
            <strong>
              {host.name}
              {badge && <span className="config-badge">{badge}</span>}
            </strong>
            <small>
              {host.username ? `${host.username}@` : ''}
              {host.hostname}
              {host.port !== 22 ? `:${host.port}` : ''}
            </small>
          </span>
          <ArrowRight size={14} className="quick-connect-arrow" />
        </button>
      ))}
    </div>
  )
}
