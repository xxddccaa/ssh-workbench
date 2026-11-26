import {
  actions,
  describe,
  keyPattern,
  type Action,
  type ActionID,
} from '../../shared/lib/keybindings'

export type Overrides = Record<string, string>

export const applePlatform =
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

const modifierOrder = ['Control', 'Meta', 'Alt', 'Shift']

function actionOf(id: string): Action | undefined {
  return actions.find((action) => action.id === id)
}

/**
 * Comparison form of a stored combination: `$mod` resolved for the platform,
 * modifiers in a fixed order and case folded, so `Shift+Alt+KeyK` and
 * `Alt+Shift+KeyK` are recognised as the same press.
 */
export function canonicalKeys(keys: string, apple = applePlatform): string {
  if (!keys) return ''
  const parts = keys.split('+')
  const key = parts.pop() ?? ''
  const modifiers = new Set(
    parts.map((part) => (part === '$mod' ? (apple ? 'Meta' : 'Control') : part)),
  )
  return [...modifierOrder.filter((modifier) => modifiers.has(modifier)), key]
    .join('+')
    .toLowerCase()
}

function sameKeys(left: string, right: string, apple: boolean): boolean {
  return canonicalKeys(left, apple) === canonicalKeys(right, apple)
}

function usable(keys: string): boolean {
  return keys === '' || keyPattern.test(keys)
}

/**
 * Keeps only overrides that differ from the defaults. Invalid values for known
 * actions are dropped (the default applies again); entries for unknown action
 * ids are preserved so a newer client's settings survive an older one.
 */
export function normalizeOverrides(overrides: Overrides, apple = applePlatform): Overrides {
  const result: Overrides = {}
  for (const [id, keys] of Object.entries(overrides)) {
    const action = actionOf(id)
    if (!action) {
      result[id] = keys
      continue
    }
    if (!usable(keys) || sameKeys(keys, action.defaultKeys, apple)) continue
    result[id] = keys
  }
  return result
}

/** Sets a binding ('' means unbound), storing it only when it differs from the default. */
export function withBinding(
  overrides: Overrides,
  id: ActionID,
  keys: string,
  apple = applePlatform,
): Overrides {
  const next = { ...overrides, [id]: keys }
  return normalizeOverrides(next, apple)
}

/** Removes the override so the action uses its default again. */
export function withoutOverride(overrides: Overrides, id: ActionID): Overrides {
  const next = { ...overrides }
  delete next[id]
  return next
}

export function isOverridden(overrides: Overrides, id: ActionID, apple = applePlatform): boolean {
  return id in normalizeOverrides(overrides, apple)
}

/** Another action whose effective binding is the same press, if any. */
export function findConflict(
  overrides: Overrides,
  id: ActionID,
  keys: string,
  apple = applePlatform,
): Action | undefined {
  if (!keys) return undefined
  return actions.find((action) => {
    if (action.id === id) return false
    const current = overrides[action.id] ?? action.defaultKeys
    return !!current && sameKeys(current, keys, apple)
  })
}

/** Rejects combinations that cannot be stored or would interfere with typing. */
export function combinationProblem(keys: string, apple = applePlatform): string | null {
  if (!keyPattern.test(keys)) return '无法识别这个组合键，请换一个。'
  const parts = keys.split('+')
  const key = parts.pop() ?? ''
  if (parts.every((part) => part === 'Shift') && !/^F\d{1,2}$/.test(key))
    return `只用 Shift 会妨碍输入大写字母和符号，请加上 ${apple ? '⌘、⌃ 或 ⌥' : 'Ctrl 或 Alt'}。`
  return null
}

export type RecordOutcome = { overrides: Overrides } | { error: string }

/** Validates a recorded combination for one action and returns the new overrides. */
export function applyRecorded(
  overrides: Overrides,
  id: ActionID,
  keys: string,
  apple = applePlatform,
): RecordOutcome {
  const problem = combinationProblem(keys, apple)
  if (problem) return { error: problem }
  const conflict = findConflict(overrides, id, keys, apple)
  if (conflict)
    return {
      error: `${describe(keys)} 已用于「${conflict.label}」。请先修改或清除那一项，再重新设置。`,
    }
  return { overrides: withBinding(overrides, id, keys, apple) }
}

/** Shown when a key without modifiers is pressed while recording. */
export function missingModifierHint(apple = applePlatform): string {
  return `快捷键需要包含修饰键：${apple ? '⌘、⌃ 或 ⌥' : 'Ctrl 或 Alt'}，可再加 Shift。`
}
