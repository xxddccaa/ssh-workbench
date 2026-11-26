import { describe, expect, it } from 'vitest'
import { parseQuickTarget } from './host-helpers'

describe('parseQuickTarget', () => {
  it('parses host, user@host, user@host:port, and ipv6 targets', () => {
    expect(parseQuickTarget('server.local')).toEqual({
      hostname: 'server.local',
      port: 22,
      username: undefined,
      authMode: 'auto',
    })
    expect(parseQuickTarget('ops@10.0.0.5:2222')).toEqual({
      hostname: '10.0.0.5',
      port: 2222,
      username: 'ops',
      authMode: 'auto',
    })
    expect(parseQuickTarget('ssh://alice@[2001:db8::1]:2200/')).toEqual({
      hostname: '2001:db8::1',
      port: 2200,
      username: 'alice',
      authMode: 'auto',
    })
  })

  it('rejects empty, whitespace, or invalid ports', () => {
    expect(parseQuickTarget('')).toBeNull()
    expect(parseQuickTarget('user@host 22')).toBeNull()
    expect(parseQuickTarget('host:99999')).toBeNull()
    expect(parseQuickTarget('@host')).toBeNull()
  })
})
