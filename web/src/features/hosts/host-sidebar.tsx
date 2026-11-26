import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  ChevronRight,
  FileInput,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Star,
  Terminal,
  Trash2,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { HostProfile, SSHConfigHost } from '../../shared/api/types'
import { useNotice } from '../../shared/ui/notices'
import { isSSHConfigHost, sshConfigToProfile } from './host-helpers'

type SidebarFilter = 'all' | 'ssh-config' | 'favorites'

export function HostSidebar({
  hosts,
  sshConfigHosts,
  sshConfigLoading,
  query,
  onQuery,
  activeHost,
  connectedHosts,
  onConnect,
  onEdit,
  onCreate,
  onImport,
  onRefreshSSHConfig,
  searchShortcut,
}: {
  hosts: HostProfile[]
  sshConfigHosts: SSHConfigHost[]
  sshConfigLoading: boolean
  query: string
  onQuery: (value: string) => void
  activeHost?: string
  connectedHosts: Set<string>
  onConnect: (host: HostProfile) => void
  onEdit: (host: HostProfile) => void
  onCreate: () => void
  onImport: () => void
  onRefreshSSHConfig: () => void
  searchShortcut: string
}) {
  const [filter, setFilter] = useState<SidebarFilter>('all')
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const queries = useQueryClient()
  const notify = useNotice()

  const sshProfiles = useMemo(
    () => sshConfigHosts.map(sshConfigToProfile),
    [sshConfigHosts],
  )

  const groups = useMemo(() => {
    const q = query.trim().toLocaleLowerCase()
    const matches = (host: HostProfile) =>
      `${host.name} ${host.hostname} ${host.username} ${host.group} ${host.proxyCommand ?? ''}`
        .toLocaleLowerCase()
        .includes(q)

    const result = new Map<string, HostProfile[]>()

    if (filter !== 'ssh-config') {
      const saved = hosts.filter(
        (host) => matches(host) && (filter !== 'favorites' || host.favorite),
      )
      for (const host of saved) {
        const group = host.favorite ? '收藏' : host.group || '已保存主机'
        result.set(group, [...(result.get(group) ?? []), host])
      }
    }

    const sorted = [...result].sort(([first], [second]) =>
      first === '收藏' ? -1 : second === '收藏' ? 1 : first.localeCompare(second, 'zh-CN'),
    )

    if (filter !== 'favorites') {
      const configFiltered = sshProfiles.filter(matches)
      if (configFiltered.length > 0) {
        sorted.push(['SSH Config (~/.ssh/config)', configFiltered])
      }
    }

    return sorted
  }, [hosts, sshProfiles, query, filter])

  const names = useMemo(
    () => new Map([...sshProfiles, ...hosts].map((host) => [host.id, host.name])),
    [hosts, sshProfiles],
  )

  async function favorite(host: HostProfile) {
    try {
      await api.saveHost({ ...host, favorite: !host.favorite }, host.id)
      await queries.invalidateQueries({ queryKey: ['hosts'] })
    } catch (problem) {
      notify(messageOf(problem))
    }
  }

  async function remove(host: HostProfile) {
    if (
      !window.confirm(
        `删除主机配置“${host.name}”？不会删除服务器上的文件，也不会自动关闭已有会话。`,
      )
    )
      return
    try {
      await api.deleteHost(host.id)
      await queries.invalidateQueries({ queryKey: ['hosts'] })
    } catch (problem) {
      notify(messageOf(problem))
    }
  }

  function toggleGroup(group: string) {
    setCollapsed((prev) => ({ ...prev, [group]: !prev[group] }))
  }

  const totalCount = hosts.length + sshProfiles.length

  return (
    <aside className="host-sidebar" aria-label="主机导航">
      <header className="sidebar-heading">
        <div className="sidebar-title">
          <h2>主机</h2>
          <span className="sidebar-count">{totalCount}</span>
        </div>
        <div className="sidebar-tools">
          <button
            className="icon-button"
            title="刷新 ~/.ssh/config"
            aria-label="刷新 SSH Config"
            onClick={onRefreshSSHConfig}
          >
            <RefreshCw size={14} className={sshConfigLoading ? 'spin' : ''} />
          </button>
          <button
            className="icon-button"
            title="导入 OpenSSH 配置"
            aria-label="导入 OpenSSH 配置"
            onClick={onImport}
          >
            <FileInput size={14} />
          </button>
          <button className="icon-button" title="新建主机" aria-label="新建主机" onClick={onCreate}>
            <Plus size={16} />
          </button>
        </div>
      </header>

      <div className="host-search">
        <Search size={13} />
        <input
          id="host-search"
          aria-label="搜索主机"
          placeholder="搜索主机、IP 或别名…"
          value={query}
          onChange={(event) => onQuery(event.target.value)}
        />
        {searchShortcut && <kbd title="可在设置 → 快捷键中修改">{searchShortcut}</kbd>}
      </div>

      <div className="sidebar-filters" role="tablist" aria-label="主机筛选">
        {(
          [
            ['all', '全部'],
            ['ssh-config', `SSH Config (${sshProfiles.length})`],
            ['favorites', '收藏'],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={filter === id}
            className={`filter-pill ${filter === id ? 'selected' : ''}`}
            onClick={() => setFilter(id)}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="host-groups">
        {groups.map(([group, entries]) => {
          const isCollapsed = !!collapsed[group]
          return (
            <section className="host-group" key={group}>
              <button
                type="button"
                className="group-toggle"
                aria-expanded={!isCollapsed}
                onClick={() => toggleGroup(group)}
              >
                {isCollapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
                <span>{group}</span>
                <em>{entries.length}</em>
              </button>
              {!isCollapsed &&
                entries.map((host) => {
                  const fromConfig = isSSHConfigHost(host)
                  const online = connectedHosts.has(host.id)
                  const endpoint = `${host.username ? `${host.username}@` : ''}${host.hostname}${host.port !== 22 ? `:${host.port}` : ''}`
                  return (
                    <div
                      className={`host-row host-${host.color} ${activeHost === host.id ? 'selected' : ''} ${host.production ? 'production' : ''}`}
                      key={`${group}-${host.id}`}
                    >
                      <button
                        className="host-row-main"
                        onClick={() => onConnect(host)}
                        title={`连接 ${endpoint}`}
                      >
                        <span className={`row-dot ${online ? 'online' : ''}`} />
                        <span className="row-text">
                          <span className="row-name">
                            <strong>{host.name}</strong>
                            {host.production && <span className="production-badge">生产</span>}
                            {fromConfig && <span className="config-badge">CONFIG</span>}
                          </span>
                          <small>
                            {endpoint}
                            {host.jumpHostId && ` · via ${names.get(host.jumpHostId) ?? '跳板'}`}
                            {host.proxyCommand && ' · ProxyCmd'}
                          </small>
                        </span>
                      </button>
                      <div className="host-row-actions">
                        {fromConfig ? (
                          <button
                            className="icon-button"
                            title="另存为可编辑主机"
                            aria-label={`编辑或保存 ${host.name}`}
                            onClick={() => onEdit(host)}
                          >
                            <Pencil size={12} />
                          </button>
                        ) : (
                          <>
                            <button
                              className={`icon-button ${host.favorite ? 'starred' : ''}`}
                              aria-label={`${host.favorite ? '取消收藏' : '收藏'} ${host.name}`}
                              onClick={() => void favorite(host)}
                            >
                              <Star size={12} />
                            </button>
                            <button
                              className="icon-button"
                              aria-label={`编辑 ${host.name}`}
                              onClick={() => onEdit(host)}
                            >
                              <Pencil size={12} />
                            </button>
                            <button
                              className="icon-button danger-hover"
                              aria-label={`删除主机 ${host.name}`}
                              onClick={() => void remove(host)}
                            >
                              <Trash2 size={12} />
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  )
                })}
            </section>
          )
        })}

        {groups.length === 0 && (
          <div className="sidebar-empty">
            {query ? <Search size={22} strokeWidth={1.5} /> : <Terminal size={22} strokeWidth={1.5} />}
            <p>{query ? '没有匹配的主机' : '暂无主机配置'}</p>
            <span>
              {query
                ? '可直接在右侧启动台输入 user@host 连接'
                : '点右上角 + 新建，或在 ~/.ssh/config 中添加服务器'}
            </span>
            {!query && (
              <button className="button secondary small" onClick={onCreate}>
                <Plus size={13} />
                新建主机
              </button>
            )}
          </div>
        )}
      </div>
    </aside>
  )
}
