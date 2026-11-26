import type { HostProfile, Tunnel, TunnelInput } from '../../shared/api/types'

export const listenHosts = ['127.0.0.1', '::1'] as const
export type ListenHost = (typeof listenHosts)[number]

/** Form state keeps ports as text so an emptied field is not silently 0. */
export interface TunnelForm {
  name: string
  hostId: string
  listenHost: ListenHost
  listenPort: string
  targetHost: string
  targetPort: string
}

export type TunnelValidation = { input: TunnelInput; error?: undefined } | { error: string }

export function emptyTunnelForm(hosts: readonly HostProfile[]): TunnelForm {
  return {
    name: '',
    hostId: hosts.length === 1 ? hosts[0].id : '',
    listenHost: '127.0.0.1',
    listenPort: '',
    targetHost: '127.0.0.1',
    targetPort: '',
  }
}

export function tunnelFormOf(tunnel: Tunnel): TunnelForm {
  return {
    name: tunnel.name,
    hostId: tunnel.hostId,
    listenHost: tunnel.listenHost,
    listenPort: String(tunnel.listenPort),
    targetHost: tunnel.targetHost,
    targetPort: String(tunnel.targetPort),
  }
}

export function parsePort(value: string): number | undefined {
  const text = value.trim()
  if (!/^\d{1,5}$/.test(text)) return undefined
  const port = Number(text)
  return port >= 1 && port <= 65535 ? port : undefined
}

/** The URL parser accepts exactly the IPv6 literals (without zone) that the server's ParseIP does. */
function isIPv6(value: string): boolean {
  try {
    new URL(`http://[${value}]/`)
    return true
  } catch {
    return false
  }
}

function isListenHost(value: string): value is ListenHost {
  return (listenHosts as readonly string[]).includes(value)
}

/**
 * Mirrors the server rules closely enough to catch typos before a round
 * trip; the server remains the authority and its message is shown as-is.
 */
export function validateTunnelForm(
  form: TunnelForm,
  hostIDs: ReadonlySet<string>,
): TunnelValidation {
  const name = form.name.trim()
  if (!name) return { error: '请填写隧道名称。' }
  if ([...name].length > 80) return { error: '隧道名称不超过 80 个字符。' }
  if (!form.hostId || !hostIDs.has(form.hostId)) return { error: '请选择 SSH 主机。' }
  if (!isListenHost(form.listenHost))
    return { error: '只能监听回环地址 127.0.0.1 或 ::1，不会对外公开。' }
  const listenPort = parsePort(form.listenPort)
  if (listenPort === undefined) return { error: '监听端口须为 1–65535 的整数。' }
  const targetHost = normalizeTargetHost(form.targetHost)
  if (!targetHost) return { error: '请填写目标主机。' }
  if (
    targetHost.length > 253 ||
    /[\s/\\@\0]/.test(targetHost) ||
    (targetHost.includes(':') && !isIPv6(targetHost))
  )
    return { error: '目标主机须为主机名或 IP 地址，不能包含空格、/、\\ 或 @。' }
  const targetPort = parsePort(form.targetPort)
  if (targetPort === undefined) return { error: '目标端口须为 1–65535 的整数。' }
  return {
    input: {
      name,
      hostId: form.hostId,
      listenHost: form.listenHost,
      listenPort,
      targetHost,
      targetPort,
    },
  }
}

/** Same normalisation as the server: surrounding whitespace and IPv6 brackets are dropped. */
export function normalizeTargetHost(value: string): string {
  return value.trim().replace(/^\[(.*)\]$/, '$1')
}

export function listenPortHint(value: string): string | undefined {
  const port = parsePort(value)
  if (port !== undefined && port < 1024)
    return '低于 1024 的端口通常需要管理员权限，启动时可能无法监听。'
  return undefined
}

/** host:port with IPv6 literals bracketed, as a user would type it. */
export function endpoint(host: string, port: number): string {
  return host.includes(':') ? `[${host}]:${port}` : `${host}:${port}`
}

/** Where the listener lives: the machine running the Go service, never the browser. */
export function listenLocationNote(remote: boolean): string {
  return remote
    ? '监听在远程服务器（Go 服务所在机器）的回环地址，不是你浏览器所在的电脑。'
    : '监听在运行 SSH Workbench 服务的这台电脑的回环地址，不对外公开。'
}
