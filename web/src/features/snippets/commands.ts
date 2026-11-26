import type { Snippet, SnippetInput } from '../../shared/api/types'

/** Same limit the server applies to stored snippets (UTF-8 bytes). */
export const commandByteLimit = 16 * 1024

const encoder = new TextEncoder()

export function byteLength(text: string): number {
  return encoder.encode(text).length
}

/** Trailing line breaks are dropped so "execute" presses Enter exactly once. */
export function trimTrailingNewlines(command: string): string {
  return command.replace(/[\r\n]+$/, '')
}

export function commandLines(command: string): string[] {
  const body = trimTrailingNewlines(command)
  return body === '' ? [] : body.split(/\r\n|\r|\n/)
}

/**
 * Text as the keyboard would type it: every line break becomes CR, which is
 * what the Enter key sends. `execute` adds one final Enter; without it the
 * last line stays at the prompt for the user to review.
 */
export function terminalInput(command: string, execute: boolean): string {
  const body = trimTrailingNewlines(command).replace(/\r\n|\r|\n/g, '\r')
  return execute ? `${body}\r` : body
}

export interface SnippetDraft {
  name: string
  command: string
  description: string
}

export type SnippetValidation = { input: SnippetInput; error?: undefined } | { error: string }

export function validateSnippet(draft: SnippetDraft): SnippetValidation {
  const name = draft.name.trim()
  if (!name) return { error: '请填写片段名称。' }
  if ([...name].length > 80) return { error: '片段名称不超过 80 个字符。' }
  if (/[\r\n\0]/.test(name)) return { error: '片段名称不能包含换行。' }
  const command = trimTrailingNewlines(draft.command)
  if (!command.trim()) return { error: '命令不能为空。' }
  if (command.includes('\0')) return { error: '命令不能包含 NUL 字符。' }
  const size = byteLength(command)
  if (size > commandByteLimit)
    return { error: `命令不超过 16 KiB（当前 ${size} 字节，上限 ${commandByteLimit} 字节）。` }
  const description = draft.description.trim()
  if ([...description].length > 200) return { error: '说明不超过 200 个字符。' }
  return { input: { name, command, description } }
}

export function filterSnippets(snippets: readonly Snippet[], query: string): Snippet[] {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return [...snippets]
  return snippets.filter((snippet) =>
    [snippet.name, snippet.command, snippet.description].some((field) =>
      field.toLocaleLowerCase().includes(needle),
    ),
  )
}

/** What a broadcast needs from a terminal; the UI's CommandTarget satisfies it. */
export interface Sendable {
  id: string
  name: string
  address: string
  production: boolean
  ready: () => boolean
  send: (text: string) => boolean
}

export interface PlannedTarget {
  id: string
  name: string
  address: string
  production: boolean
}

/** An immutable snapshot of exactly what the user previewed and will confirm. */
export interface BroadcastPlan {
  command: string
  payload: string
  lineCount: number
  targets: readonly PlannedTarget[]
  productionCount: number
}

export type PlanResult = { plan: BroadcastPlan; error?: undefined } | { error: string }

export function planBroadcast(
  command: string,
  targets: readonly Sendable[],
  selected: ReadonlySet<string>,
): PlanResult {
  const body = trimTrailingNewlines(command)
  if (!body.trim()) return { error: '请输入要广播的命令。' }
  if (body.includes('\0')) return { error: '命令不能包含 NUL 字符。' }
  const size = byteLength(body)
  if (size > commandByteLimit)
    return { error: `命令不超过 16 KiB（当前 ${size} 字节，上限 ${commandByteLimit} 字节）。` }
  const chosen = targets
    .filter((target) => selected.has(target.id))
    .map(({ id, name, address, production }) => ({ id, name, address, production }))
  if (chosen.length === 0) return { error: '请至少勾选一台目标主机。' }
  return {
    plan: {
      command: body,
      payload: terminalInput(body, true),
      lineCount: commandLines(body).length,
      targets: chosen,
      productionCount: chosen.filter((target) => target.production).length,
    },
  }
}

/** Accepts full-width digits typed through a Chinese IME. */
export function confirmationMatches(typed: string, count: number): boolean {
  return typed.normalize('NFKC').trim() === String(count)
}

export type BroadcastOutcome = 'sent' | 'not-ready' | 'closed' | 'error'

export interface BroadcastResult extends PlannedTarget {
  outcome: BroadcastOutcome
}

export const outcomeLabel: Record<BroadcastOutcome, string> = {
  sent: '已发送',
  'not-ready': '未发送（连接未就绪）',
  closed: '未发送（终端已关闭）',
  error: '发送出错，可能只发送了部分内容',
}

/**
 * Sends the payload once to each planned target, in order, re-resolving each
 * terminal at send time. Nothing is retried or queued.
 */
export function runBroadcast(
  plan: BroadcastPlan,
  resolve: (id: string) => Sendable | undefined,
): BroadcastResult[] {
  return plan.targets.map((target) => {
    const live = resolve(target.id)
    let outcome: BroadcastOutcome
    if (!live) outcome = 'closed'
    else if (!live.ready()) outcome = 'not-ready'
    else {
      try {
        outcome = live.send(plan.payload) ? 'sent' : 'not-ready'
      } catch {
        outcome = 'error'
      }
    }
    return { ...target, outcome }
  })
}

/** Runs a plan at most once per `ledger`; a repeated call returns undefined. */
export function sendOnce(
  plan: BroadcastPlan,
  ledger: WeakSet<BroadcastPlan>,
  resolve: (id: string) => Sendable | undefined,
): BroadcastResult[] | undefined {
  if (ledger.has(plan)) return undefined
  ledger.add(plan)
  return runBroadcast(plan, resolve)
}

export function tally(results: readonly BroadcastResult[]) {
  const sent = results.filter((result) => result.outcome === 'sent').length
  return { sent, unsent: results.length - sent }
}
