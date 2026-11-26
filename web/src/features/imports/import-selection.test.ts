import { describe, expect, it } from 'vitest'
import type { ImportCandidate, ImportDiagnostic } from '../../shared/api/types'
import {
  allSelectable,
  contentProblem,
  diagnosticLocation,
  formatEndpoint,
  formatJump,
  implicitJumps,
  previewSummary,
  selectable,
  selectedAliases,
  sortDiagnostics,
  toggled,
  worstLevel,
} from './import-selection'

function candidate(alias: string, extra: Partial<ImportCandidate> = {}): ImportCandidate {
  return {
    alias,
    hostname: `${alias}.example.test`,
    port: 22,
    user: 'dev',
    identitiesOnly: false,
    useAgent: false,
    importable: true,
    diagnostics: [],
    existing: false,
    ...extra,
  }
}

const candidates = [
  candidate('bastion'),
  candidate('app', { jump: { alias: 'bastion', hostname: 'bastion.example.test', port: 22 } }),
  candidate('db', { jump: { user: 'ops', hostname: '10.0.0.1', port: 2222 } }),
  candidate('old', { existing: true }),
  candidate('proxy', { importable: false }),
]

describe('import selection', () => {
  it('only allows importable hosts that do not exist yet', () => {
    expect(candidates.filter((item) => selectable(item)).map((item) => item.alias)).toEqual([
      'bastion',
      'app',
      'db',
    ])
    expect([...allSelectable(candidates)]).toEqual(['bastion', 'app', 'db'])
  })
  it('excludes hosts without User unless the server can use its own user name', () => {
    const userless = [candidate('a'), candidate('b', { user: '' })]
    expect([...allSelectable(userless, true)]).toEqual(['a', 'b'])
    expect([...allSelectable(userless, false)]).toEqual(['a'])
    expect(selectedAliases(new Set(['a', 'b']), userless, false)).toEqual(['a'])
    expect(previewSummary({ candidates: userless, diagnostics: [] }, false)).toBe(
      '找到 2 个主机，1 个可导入，1 个缺少 User。',
    )
  })
  it('keeps file order and ignores stale or blocked aliases', () => {
    const selection = new Set(['db', 'proxy', 'old', 'gone', 'app'])
    expect(selectedAliases(selection, candidates)).toEqual(['app', 'db'])
  })
  it('toggles without mutating the previous selection', () => {
    const before = new Set(['app'])
    const after = toggled(before, 'db', true)
    expect([...before]).toEqual(['app'])
    expect([...after]).toEqual(['app', 'db'])
    expect([...toggled(after, 'app', false)]).toEqual(['db'])
  })
  it('lists jump hosts that come along with the selection', () => {
    expect(implicitJumps(new Set(['app']), candidates)).toEqual(['bastion'])
    expect(implicitJumps(new Set(['app', 'bastion']), candidates)).toEqual([])
    expect(implicitJumps(new Set(['db']), candidates)).toEqual(['ops@10.0.0.1:2222'])
    expect(implicitJumps(new Set(), candidates)).toEqual([])
  })
  it('summarises the preview', () => {
    expect(previewSummary({ candidates, diagnostics: [] })).toBe(
      '找到 5 个主机，3 个可导入，1 个已存在，1 个不可导入。',
    )
    expect(previewSummary({ candidates: [], diagnostics: [] })).toContain('没有')
  })
})

describe('import formatting', () => {
  it('formats endpoints and jumps', () => {
    expect(formatEndpoint('dev', 'example.test', 22)).toBe('dev@example.test:22')
    expect(formatEndpoint('', 'example.test', 2200)).toBe('example.test:2200')
    expect(formatEndpoint('dev', '2001:db8::1', 22)).toBe('dev@[2001:db8::1]:22')
    expect(formatJump({ alias: 'bastion', hostname: 'b.example.test', port: 22 })).toBe('bastion')
    expect(formatJump({ hostname: 'b.example.test', port: 22 })).toBe('b.example.test:22')
  })
  it('orders diagnostics by severity, keeping file order within a level', () => {
    const diagnostics: ImportDiagnostic[] = [
      { level: 'info', directive: 'A', message: '1' },
      { level: 'warning', directive: 'B', message: '2' },
      { level: 'blocked', directive: 'C', message: '3' },
      { level: 'warning', directive: 'D', message: '4' },
    ]
    expect(sortDiagnostics(diagnostics).map((item) => item.directive)).toEqual(['C', 'B', 'D', 'A'])
    expect(worstLevel(diagnostics)).toBe('blocked')
    expect(worstLevel([])).toBeNull()
  })
  it('describes where a diagnostic comes from', () => {
    expect(diagnosticLocation({ level: 'info', directive: 'X', message: '', line: 4 })).toBe(
      '第 4 行',
    )
    expect(
      diagnosticLocation({
        level: 'info',
        directive: 'X',
        message: '',
        source: '~/.ssh/config',
        line: 4,
      }),
    ).toBe('~/.ssh/config:4')
    expect(diagnosticLocation({ level: 'info', directive: 'X', message: '' })).toBe('')
  })
  it('validates pasted content size', () => {
    expect(contentProblem('  \n')).not.toBeNull()
    expect(contentProblem('Host a\n  HostName a.example.test\n')).toBeNull()
    expect(contentProblem('中'.repeat(400_000))).toContain('1 MiB')
  })
})
