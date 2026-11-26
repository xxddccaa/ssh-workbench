import { describe, expect, it } from 'vitest'
import type { HostProfile } from '../../shared/api/types'
import {
  emptyTunnelForm,
  endpoint,
  listenLocationNote,
  parsePort,
  validateTunnelForm,
  type TunnelForm,
} from './tunnel-form'

const hosts = new Set(['h1'])
const valid: TunnelForm = {
  name: ' 数据库 ',
  hostId: 'h1',
  listenHost: '127.0.0.1',
  listenPort: '15432',
  targetHost: ' db.internal ',
  targetPort: '5432',
}

describe('tunnel form', () => {
  it('parses ports strictly within 1–65535', () => {
    expect(parsePort('1')).toBe(1)
    expect(parsePort(' 65535 ')).toBe(65535)
    for (const bad of ['', '0', '65536', '-1', '1.5', '1e3', 'abc', '123456'])
      expect(parsePort(bad)).toBeUndefined()
  })

  it('builds trimmed input from a valid form', () => {
    expect(validateTunnelForm(valid, hosts)).toEqual({
      input: {
        name: '数据库',
        hostId: 'h1',
        listenHost: '127.0.0.1',
        listenPort: 15432,
        targetHost: 'db.internal',
        targetPort: 5432,
      },
    })
    const ipv6 = validateTunnelForm({ ...valid, listenHost: '::1', targetHost: '[::1]' }, hosts)
    expect(ipv6.error).toBeUndefined()
    if (ipv6.error === undefined) expect(ipv6.input.targetHost).toBe('::1')
  })

  it('rejects missing fields, unknown hosts, bad ports and unsafe targets', () => {
    expect(validateTunnelForm({ ...valid, name: '  ' }, hosts).error).toBeTruthy()
    expect(validateTunnelForm({ ...valid, hostId: '' }, hosts).error).toBeTruthy()
    expect(validateTunnelForm({ ...valid, hostId: 'deleted' }, hosts).error).toBeTruthy()
    expect(validateTunnelForm({ ...valid, listenPort: '0' }, hosts).error).toContain('监听端口')
    expect(validateTunnelForm({ ...valid, targetPort: '70000' }, hosts).error).toContain('目标端口')
    expect(validateTunnelForm({ ...valid, targetHost: '' }, hosts).error).toBeTruthy()
    for (const target of ['a b', 'user@db', 'db/x', 'db:5432'])
      expect(validateTunnelForm({ ...valid, targetHost: target }, hosts).error).toBeTruthy()
    const publicListen = { ...valid, listenHost: '0.0.0.0' } as unknown as TunnelForm
    expect(validateTunnelForm(publicListen, hosts).error).toContain('回环')
  })

  it('defaults to IPv4 loopback and preselects a lone host', () => {
    const form = emptyTunnelForm([{ id: 'only' } as HostProfile])
    expect(form.listenHost).toBe('127.0.0.1')
    expect(form.targetHost).toBe('127.0.0.1')
    expect(form.hostId).toBe('only')
    expect(emptyTunnelForm([]).hostId).toBe('')
  })

  it('formats endpoints and explains where the listener lives', () => {
    expect(endpoint('127.0.0.1', 8080)).toBe('127.0.0.1:8080')
    expect(endpoint('::1', 8080)).toBe('[::1]:8080')
    expect(listenLocationNote(false)).toContain('这台电脑')
    expect(listenLocationNote(true)).toContain('不是你浏览器所在的电脑')
  })
})
