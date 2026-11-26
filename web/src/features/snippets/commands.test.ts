import { describe, expect, it } from 'vitest'
import {
  commandByteLimit,
  commandLines,
  confirmationMatches,
  filterSnippets,
  planBroadcast,
  runBroadcast,
  sendOnce,
  tally,
  terminalInput,
  validateSnippet,
  type BroadcastPlan,
  type Sendable,
} from './commands'

function target(
  id: string,
  options: { ready?: boolean; production?: boolean; accept?: boolean } = {},
) {
  const sent: string[] = []
  const value: Sendable = {
    id,
    name: `host-${id}`,
    address: `root@${id}.example:22`,
    production: options.production ?? false,
    ready: () => options.ready ?? true,
    send: (text) => {
      if (options.accept === false) return false
      sent.push(text)
      return true
    },
  }
  return { value, sent }
}

const snippet = (name: string, command: string, description = '') => ({
  id: name,
  name,
  command,
  description,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

describe('terminal input', () => {
  it('types line breaks as Enter and presses Enter once when executing', () => {
    expect(terminalInput('ls -la', false)).toBe('ls -la')
    expect(terminalInput('ls -la\n\n', true)).toBe('ls -la\r')
    expect(terminalInput('cd /tmp\r\nls\npwd', true)).toBe('cd /tmp\rls\rpwd\r')
  })

  it('counts lines without trailing blank lines', () => {
    expect(commandLines('')).toEqual([])
    expect(commandLines('a\r\nb\n')).toEqual(['a', 'b'])
  })
})

describe('snippet validation', () => {
  it('trims and accepts a valid snippet', () => {
    expect(validateSnippet({ name: ' 磁盘 ', command: 'df -h\n', description: ' ' })).toEqual({
      input: { name: '磁盘', command: 'df -h', description: '' },
    })
  })

  it('rejects empty fields and oversize commands in UTF-8 bytes', () => {
    expect(validateSnippet({ name: '', command: 'x', description: '' }).error).toBeTruthy()
    expect(validateSnippet({ name: 'a', command: ' \n ', description: '' }).error).toBeTruthy()
    expect(
      validateSnippet({ name: 'x'.repeat(81), command: 'x', description: '' }).error,
    ).toBeTruthy()
    const wide = '中'.repeat(Math.floor(commandByteLimit / 3) + 1)
    expect(wide.length).toBeLessThan(commandByteLimit)
    expect(validateSnippet({ name: 'a', command: wide, description: '' }).error).toContain('16 KiB')
    expect(
      validateSnippet({ name: 'a', command: 'x', description: '说'.repeat(201) }).error,
    ).toBeTruthy()
  })

  it('filters by name, command and description case-insensitively', () => {
    const list = [snippet('Disk', 'df -h'), snippet('日志', 'journalctl -f', 'Follow logs')]
    expect(filterSnippets(list, 'disk').map((item) => item.name)).toEqual(['Disk'])
    expect(filterSnippets(list, 'JOURNAL').map((item) => item.name)).toEqual(['日志'])
    expect(filterSnippets(list, 'follow').map((item) => item.name)).toEqual(['日志'])
    expect(filterSnippets(list, '  ')).toHaveLength(2)
  })
})

describe('broadcast planning', () => {
  const a = target('a')
  const b = target('b', { production: true })
  const c = target('c')

  it('requires a command and at least one explicitly selected target', () => {
    expect(planBroadcast('  ', [a.value], new Set(['a'])).error).toBeTruthy()
    expect(planBroadcast('uptime', [a.value], new Set()).error).toBeTruthy()
    expect(planBroadcast('uptime', [a.value], new Set(['gone'])).error).toBeTruthy()
  })

  it('snapshots selected targets in display order with the exact payload', () => {
    const result = planBroadcast('uptime\nwho\n', [a.value, b.value, c.value], new Set(['c', 'b']))
    if (result.error !== undefined) throw new Error(result.error)
    expect(result.plan.command).toBe('uptime\nwho')
    expect(result.plan.payload).toBe('uptime\rwho\r')
    expect(result.plan.lineCount).toBe(2)
    expect(result.plan.productionCount).toBe(1)
    expect(result.plan.targets.map((item) => item.id)).toEqual(['b', 'c'])
    expect(result.plan.targets[0]).not.toHaveProperty('send')
  })

  it('accepts the confirmation count typed with full-width digits', () => {
    expect(confirmationMatches(' 3 ', 3)).toBe(true)
    expect(confirmationMatches('３', 3)).toBe(true)
    expect(confirmationMatches('2', 3)).toBe(false)
    expect(confirmationMatches('', 3)).toBe(false)
  })
})

describe('broadcast execution', () => {
  function plan(targets: Sendable[]): BroadcastPlan {
    const result = planBroadcast('uptime', targets, new Set(targets.map((item) => item.id)))
    if (result.error !== undefined) throw new Error(result.error)
    return result.plan
  }

  it('sends once per ready target, in order, and records why others were skipped', () => {
    const ready = target('ready')
    const idle = target('idle', { ready: false })
    const full = target('full', { accept: false })
    const closed = target('closed')
    const throwing: Sendable = {
      ...target('throws').value,
      send: () => {
        throw new Error('socket')
      },
    }
    const order: string[] = []
    const live = new Map(
      [ready.value, idle.value, full.value, throwing].map((item) => [item.id, item]),
    )
    const broadcast = plan([ready.value, idle.value, full.value, closed.value, throwing])
    const results = runBroadcast(broadcast, (id) => {
      order.push(id)
      return live.get(id)
    })
    expect(order).toEqual(['ready', 'idle', 'full', 'closed', 'throws'])
    expect(results.map((result) => result.outcome)).toEqual([
      'sent',
      'not-ready',
      'not-ready',
      'closed',
      'error',
    ])
    expect(ready.sent).toEqual(['uptime\r'])
    expect(idle.sent).toEqual([])
    expect(tally(results)).toEqual({ sent: 1, unsent: 4 })
  })

  it('never sends the same plan twice', () => {
    const one = target('one')
    const two = target('two')
    const ledger = new WeakSet<BroadcastPlan>()
    const broadcast = plan([one.value, two.value])
    const resolve = (id: string) => (id === 'one' ? one.value : two.value)
    expect(sendOnce(broadcast, ledger, resolve)).toHaveLength(2)
    expect(sendOnce(broadcast, ledger, resolve)).toBeUndefined()
    expect(one.sent).toEqual(['uptime\r'])
    expect(two.sent).toEqual(['uptime\r'])
    const again = plan([one.value])
    expect(sendOnce(again, ledger, resolve)).toHaveLength(1)
    expect(one.sent).toHaveLength(2)
  })
})
