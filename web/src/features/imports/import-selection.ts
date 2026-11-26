import type { ImportCandidate, ImportDiagnostic, ImportPreview } from '../../shared/api/types'

export type ImportJump = NonNullable<ImportCandidate['jump']>
export type DiagnosticLevel = ImportDiagnostic['level']

export const contentLimit = 1024 * 1024
export const defaultGroup = 'SSH 配置导入'

const encoder = new TextEncoder()

export function contentProblem(content: string): string | null {
  if (!content.trim()) return '请先粘贴配置内容。'
  if (encoder.encode(content).length > contentLimit) return '配置内容不得超过 1 MiB。'
  return null
}

/**
 * Hosts that can be ticked: parsed cleanly, not already in the host list, and
 * with a login user. Only a local import can fall back to the server's own
 * user name; pasted or uploaded entries without User are always skipped.
 */
export function selectable(candidate: ImportCandidate, local = true): boolean {
  return candidate.importable && !candidate.existing && (local || !!candidate.user)
}

export function allSelectable(candidates: ImportCandidate[], local = true): Set<string> {
  return new Set(
    candidates
      .filter((candidate) => selectable(candidate, local))
      .map((candidate) => candidate.alias),
  )
}

export function toggled(selection: Set<string>, alias: string, on: boolean): Set<string> {
  const next = new Set(selection)
  if (on) next.add(alias)
  else next.delete(alias)
  return next
}

/** Selected aliases in file order; stale or non-selectable entries are ignored. */
export function selectedAliases(
  selection: Set<string>,
  candidates: ImportCandidate[],
  local = true,
): string[] {
  return candidates
    .filter((candidate) => selectable(candidate, local) && selection.has(candidate.alias))
    .map((candidate) => candidate.alias)
}

/**
 * Jump hosts that will be imported or linked automatically because a selected
 * host names them in ProxyJump but they are not selected themselves.
 */
export function implicitJumps(
  selection: Set<string>,
  candidates: ImportCandidate[],
  local = true,
): string[] {
  const chosen = new Set(selectedAliases(selection, candidates, local))
  const result: string[] = []
  for (const candidate of candidates) {
    if (!chosen.has(candidate.alias) || !candidate.jump) continue
    const name = candidate.jump.alias || formatJump(candidate.jump)
    if (!chosen.has(name) && !result.includes(name)) result.push(name)
  }
  return result
}

function bracketed(hostname: string): string {
  return hostname.includes(':') && !hostname.startsWith('[') ? `[${hostname}]` : hostname
}

export function formatEndpoint(user: string | undefined, hostname: string, port: number): string {
  return `${user ? `${user}@` : ''}${bracketed(hostname)}:${port}`
}

export function formatJump(jump: ImportJump): string {
  return jump.alias || formatEndpoint(jump.user, jump.hostname, jump.port)
}

const severity: Record<DiagnosticLevel, number> = { blocked: 0, warning: 1, info: 2 }

/** Most severe first; equal levels keep their original order. */
export function sortDiagnostics(diagnostics: ImportDiagnostic[]): ImportDiagnostic[] {
  return diagnostics
    .map((diagnostic, index) => ({ diagnostic, index }))
    .sort(
      (left, right) =>
        severity[left.diagnostic.level] - severity[right.diagnostic.level] ||
        left.index - right.index,
    )
    .map(({ diagnostic }) => diagnostic)
}

export function worstLevel(diagnostics: ImportDiagnostic[]): DiagnosticLevel | null {
  return sortDiagnostics(diagnostics)[0]?.level ?? null
}

export function diagnosticLocation(diagnostic: ImportDiagnostic): string {
  if (!diagnostic.source) return diagnostic.line ? `第 ${diagnostic.line} 行` : ''
  return diagnostic.line ? `${diagnostic.source}:${diagnostic.line}` : diagnostic.source
}

export const levelLabels: Record<DiagnosticLevel, string> = {
  blocked: '已阻止',
  warning: '注意',
  info: '说明',
}

export function previewSummary(preview: ImportPreview, local = true): string {
  const total = preview.candidates.length
  if (total === 0) return '没有找到可列出的 Host 条目。'
  const ready = preview.candidates.filter((candidate) => selectable(candidate, local)).length
  const existing = preview.candidates.filter((candidate) => candidate.existing).length
  const blocked = preview.candidates.filter((candidate) => !candidate.importable).length
  const userless = preview.candidates.filter(
    (candidate) => !local && candidate.importable && !candidate.existing && !candidate.user,
  ).length
  const parts = [`找到 ${total} 个主机，${ready} 个可导入`]
  if (existing) parts.push(`${existing} 个已存在`)
  if (blocked) parts.push(`${blocked} 个不可导入`)
  if (userless) parts.push(`${userless} 个缺少 User`)
  return `${parts.join('，')}。`
}
