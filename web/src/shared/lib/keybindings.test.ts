import { describe, expect, it } from 'vitest'
import { actionFor, bindings, keyPattern } from './keybindings'

// Vitest runs in Node, which has no KeyboardEvent; this models the fields
// tinykeys and the composition guard read.
function key(init: {
  code: string
  key: string
  altKey?: boolean
  shiftKey?: boolean
  ctrlKey?: boolean
  metaKey?: boolean
  isComposing?: boolean
  keyCode?: number
}) {
  const modifiers: Record<string, boolean> = {
    Alt: !!init.altKey,
    Shift: !!init.shiftKey,
    Control: !!init.ctrlKey,
    Meta: !!init.metaKey,
  }
  return {
    ...init,
    isComposing: !!init.isComposing,
    keyCode: init.keyCode ?? 0,
    getModifierState: (modifier: string) => modifiers[modifier] ?? false,
  } as unknown as KeyboardEvent
}

describe('keyboard shortcuts', () => {
  const current = bindings({ 'files.toggle': '', 'tab.next': 'Alt+Shift+KeyJ' })

  it('matches configured combinations by physical key', () => {
    expect(
      actionFor(key({ code: 'Digit2', key: '@', altKey: true, shiftKey: true }), current),
    ).toBe('layout.columns')
    expect(actionFor(key({ code: 'KeyJ', key: 'J', altKey: true, shiftKey: true }), current)).toBe(
      'tab.next',
    )
  })

  it('never fires during IME composition or for unbound actions', () => {
    expect(
      actionFor(
        key({ code: 'Digit2', key: '@', altKey: true, shiftKey: true, isComposing: true }),
        current,
      ),
    ).toBeUndefined()
    expect(
      actionFor(key({ code: 'Digit2', key: 'Process', altKey: true, shiftKey: true }), current),
    ).toBeUndefined()
    expect(
      actionFor(
        key({ code: 'Digit2', key: '2', altKey: true, shiftKey: true, keyCode: 229 }),
        current,
      ),
    ).toBeUndefined()
    expect(
      actionFor(key({ code: 'KeyF', key: 'F', altKey: true, shiftKey: true }), current),
    ).toBeUndefined()
    expect(actionFor(key({ code: 'KeyA', key: 'a' }), current)).toBeUndefined()
  })

  it('defaults are valid stored combinations', () => {
    for (const value of Object.values(bindings())) expect(keyPattern.test(value)).toBe(true)
  })
})
