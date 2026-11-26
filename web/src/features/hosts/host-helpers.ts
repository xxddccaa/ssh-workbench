import type { HostProfile, QuickConnectTarget, SSHConfigHost } from '../../shared/api/types'

export function isSSHConfigHost(host: HostProfile): boolean {
  return host.id.startsWith('sshcfg:')
}

export function isQuickHost(host: HostProfile): boolean {
  return host.id.startsWith('quick:')
}

export function sshConfigToProfile(entry: SSHConfigHost): HostProfile {
  const now = new Date(0).toISOString()
  return {
    id: entry.id,
    name: entry.alias,
    hostname: entry.hostname,
    port: entry.port,
    username: entry.user || '',
    authMode: 'auto',
    group: 'SSH Config',
    color: 'teal',
    favorite: false,
    production: false,
    proxyCommand: entry.proxyCommand,
    identityFile: entry.identityFiles?.[0],
    createdAt: now,
    updatedAt: now,
  }
}

export function quickTargetToProfile(target: QuickConnectTarget): HostProfile {
  const now = new Date(0).toISOString()
  const username = target.username || ''
  const port = target.port || 22
  return {
    id: `quick:${username ? `${username}@` : ''}${target.hostname}:${port}`,
    name: target.name || (username ? `${username}@${target.hostname}` : target.hostname),
    hostname: target.hostname,
    port,
    username,
    authMode: target.authMode || 'auto',
    group: '快速连接',
    color: 'teal',
    favorite: false,
    production: false,
    proxyCommand: target.proxyCommand,
    identityFile: target.identityFile,
    createdAt: now,
    updatedAt: now,
  }
}

/**
 * Parses `host`, `user@host`, `host:port`, `user@host:port`, or `[ipv6]:port`.
 * Returns null if the string is empty or contains whitespace.
 */
export function parseQuickTarget(raw: string): QuickConnectTarget | null {
  let value = raw.trim()
  if (!value || /\s/.test(value)) return null
  if (value.startsWith('ssh://')) value = value.slice('ssh://'.length).replace(/\/+$/, '')

  let username: string | undefined
  const atIndex = value.lastIndexOf('@')
  if (atIndex > 0) {
    username = value.slice(0, atIndex)
    value = value.slice(atIndex + 1)
  } else if (atIndex === 0) {
    return null
  }

  let hostname = value
  let port = 22
  if (value.startsWith('[')) {
    const close = value.indexOf(']')
    if (close < 0) return null
    hostname = value.slice(1, close)
    const rest = value.slice(close + 1)
    if (rest.startsWith(':')) {
      const parsed = Number(rest.slice(1))
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535) return null
      port = parsed
    } else if (rest !== '') {
      return null
    }
  } else {
    const firstColon = value.indexOf(':')
    const lastColon = value.lastIndexOf(':')
    if (firstColon > 0 && firstColon === lastColon) {
      const parsed = Number(value.slice(lastColon + 1))
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535) return null
      hostname = value.slice(0, lastColon)
      port = parsed
    }
  }

  if (!hostname) return null
  return {
    hostname,
    port,
    username,
    authMode: 'auto',
  }
}
