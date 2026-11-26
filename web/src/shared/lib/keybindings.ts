import { matchKeybindingPress, parseKeybinding } from 'tinykeys'

export type ActionID =
  | 'hosts.search'
  | 'connect.quick'
  | 'tab.next'
  | 'tab.previous'
  | 'tab.close'
  | 'pane.next'
  | 'layout.single'
  | 'layout.columns'
  | 'layout.rows'
  | 'layout.grid'
  | 'files.toggle'
  | 'transfers.toggle'
  | 'tunnels.toggle'
  | 'terminal.search'
  | 'snippets.open'
  | 'broadcast.open'
  | 'font.larger'
  | 'font.smaller'

export interface Action {
  id: ActionID
  label: string
  defaultKeys: string
}

/**
 * Defaults use Alt+Shift so they rarely collide with shell, editor or
 * browser-reserved shortcuts (Ctrl+W, Cmd+T…). Browsers keep some keys for
 * themselves no matter what a page does; those cannot be bound here.
 */
export const actions: Action[] = [
  { id: 'hosts.search', label: '搜索主机', defaultKeys: 'Alt+Shift+KeyK' },
  { id: 'connect.quick', label: '新建主机连接', defaultKeys: 'Alt+Shift+KeyN' },
  { id: 'tab.next', label: '下一个标签', defaultKeys: 'Alt+Shift+ArrowRight' },
  { id: 'tab.previous', label: '上一个标签', defaultKeys: 'Alt+Shift+ArrowLeft' },
  { id: 'tab.close', label: '关闭当前标签', defaultKeys: 'Alt+Shift+KeyW' },
  { id: 'pane.next', label: '切换到下一个分屏', defaultKeys: 'Alt+Shift+KeyO' },
  { id: 'layout.single', label: '单屏', defaultKeys: 'Alt+Shift+Digit1' },
  { id: 'layout.columns', label: '左右分屏', defaultKeys: 'Alt+Shift+Digit2' },
  { id: 'layout.rows', label: '上下分屏', defaultKeys: 'Alt+Shift+Digit3' },
  { id: 'layout.grid', label: '四宫格', defaultKeys: 'Alt+Shift+Digit4' },
  { id: 'files.toggle', label: '显示/隐藏文件面板', defaultKeys: 'Alt+Shift+KeyF' },
  { id: 'transfers.toggle', label: '显示/隐藏传输任务', defaultKeys: 'Alt+Shift+KeyU' },
  { id: 'tunnels.toggle', label: '显示/隐藏隧道', defaultKeys: 'Alt+Shift+KeyL' },
  { id: 'terminal.search', label: '搜索终端输出', defaultKeys: 'Alt+Shift+KeyS' },
  { id: 'snippets.open', label: '命令片段', defaultKeys: 'Alt+Shift+KeyP' },
  { id: 'broadcast.open', label: '多主机广播', defaultKeys: 'Alt+Shift+KeyB' },
  { id: 'font.larger', label: '增大字号', defaultKeys: 'Alt+Shift+Equal' },
  { id: 'font.smaller', label: '减小字号', defaultKeys: 'Alt+Shift+Minus' },
]

export const keyPattern = /^(\$mod\+|Control\+|Meta\+|Alt\+|Shift\+){1,3}[A-Za-z0-9]{1,12}$/

export function bindings(overrides: Record<string, string> = {}): Record<ActionID, string> {
  const result = {} as Record<ActionID, string>
  for (const action of actions) {
    const override = overrides[action.id]
    result[action.id] = override === undefined ? action.defaultKeys : override
  }
  return result
}

export function matches(event: KeyboardEvent, keys: string): boolean {
  if (!keys) return false
  try {
    const presses = parseKeybinding(keys)
    return presses.length === 1 && matchKeybindingPress(event, presses[0])
  } catch {
    return false
  }
}

/** Finds the action bound to a key event, ignoring IME composition. */
export function actionFor(
  event: KeyboardEvent,
  current: Record<ActionID, string>,
): ActionID | undefined {
  if (event.isComposing || event.key === 'Process' || event.keyCode === 229) return undefined
  return actions.find((action) => matches(event, current[action.id]))?.id
}

const isApple = /Mac|iPhone|iPad/.test(navigator.platform)

export function describe(keys: string): string {
  if (!keys) return '未设置'
  return keys
    .split('+')
    .map((part) => {
      if (part === '$mod') return isApple ? '⌘' : 'Ctrl'
      if (part === 'Meta') return isApple ? '⌘' : 'Win'
      if (part === 'Alt') return isApple ? '⌥' : 'Alt'
      if (part === 'Shift') return isApple ? '⇧' : 'Shift'
      if (part === 'Control') return isApple ? '⌃' : 'Ctrl'
      return part
        .replace(/^Key/, '')
        .replace(/^Digit/, '')
        .replace('ArrowRight', '→')
        .replace('ArrowLeft', '←')
        .replace('ArrowUp', '↑')
        .replace('ArrowDown', '↓')
        .replace('Equal', '=')
        .replace('Minus', '-')
    })
    .join(isApple ? '' : '+')
}

/** Converts a pressed combination into the stored form, e.g. Alt+Shift+KeyK. */
export function record(event: KeyboardEvent): string | null {
  if (['Shift', 'Control', 'Alt', 'Meta', 'Process'].includes(event.key) || event.isComposing)
    return null
  const parts: string[] = []
  if (isApple ? event.metaKey : event.ctrlKey) parts.push('$mod')
  if (isApple && event.ctrlKey) parts.push('Control')
  if (!isApple && event.metaKey) parts.push('Meta')
  if (event.altKey) parts.push('Alt')
  if (event.shiftKey) parts.push('Shift')
  if (parts.length === 0) return null
  const combination = [...parts.slice(0, 3), event.code].join('+')
  return keyPattern.test(combination) ? combination : null
}
