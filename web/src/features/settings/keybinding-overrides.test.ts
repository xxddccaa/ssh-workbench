import { describe, expect, it } from 'vitest'
import { actions, bindings } from '../../shared/lib/keybindings'
import {
  applyRecorded,
  canonicalKeys,
  combinationProblem,
  findConflict,
  isOverridden,
  normalizeOverrides,
  withBinding,
  withoutOverride,
} from './keybinding-overrides'

const search = actions.find((action) => action.id === 'hosts.search')!

describe('keybinding comparison', () => {
  it('ignores modifier order and case', () => {
    expect(canonicalKeys('Shift+Alt+KeyK', false)).toBe(canonicalKeys('Alt+Shift+keyk', false))
  })
  it('resolves $mod per platform', () => {
    expect(canonicalKeys('$mod+KeyJ', true)).toBe(canonicalKeys('Meta+KeyJ', true))
    expect(canonicalKeys('$mod+KeyJ', false)).toBe(canonicalKeys('Control+KeyJ', false))
    expect(canonicalKeys('$mod+KeyJ', false)).not.toBe(canonicalKeys('Meta+KeyJ', false))
  })
})

describe('override normalization', () => {
  it('stores only values that differ from the default', () => {
    expect(withBinding({}, 'hosts.search', search.defaultKeys, false)).toEqual({})
    expect(withBinding({}, 'hosts.search', 'Shift+Alt+KeyK', false)).toEqual({})
    expect(withBinding({}, 'hosts.search', 'Alt+Shift+KeyJ', false)).toEqual({
      'hosts.search': 'Alt+Shift+KeyJ',
    })
  })
  it('stores an unbound action as an empty string', () => {
    const overrides = withBinding({}, 'tab.close', '', false)
    expect(overrides).toEqual({ 'tab.close': '' })
    expect(bindings(overrides)['tab.close']).toBe('')
  })
  it('restores a default by removing the override', () => {
    const overrides = { 'hosts.search': 'Alt+Shift+KeyJ', 'tab.close': '' }
    expect(withoutOverride(overrides, 'tab.close')).toEqual({ 'hosts.search': 'Alt+Shift+KeyJ' })
    expect(isOverridden(overrides, 'tab.close', false)).toBe(true)
    expect(isOverridden({}, 'tab.close', false)).toBe(false)
  })
  it('drops invalid values but keeps unknown action ids', () => {
    expect(
      normalizeOverrides({ 'hosts.search': 'not a key', 'future.action': 'Alt+KeyZ' }, false),
    ).toEqual({ 'future.action': 'Alt+KeyZ' })
  })
})

describe('recording', () => {
  it('detects conflicts with the effective binding of other actions', () => {
    expect(findConflict({}, 'hosts.search', 'Alt+Shift+KeyN', false)?.id).toBe('connect.quick')
    expect(findConflict({}, 'hosts.search', 'Shift+Alt+KeyN', false)?.id).toBe('connect.quick')
    expect(findConflict({ 'connect.quick': '' }, 'hosts.search', 'Alt+Shift+KeyN', false)).toBe(
      undefined,
    )
    expect(findConflict({}, 'hosts.search', 'Alt+Shift+KeyK', false)).toBe(undefined)
    expect(findConflict({}, 'hosts.search', '', false)).toBe(undefined)
  })
  it('returns an error instead of saving a conflicting binding', () => {
    const outcome = applyRecorded({}, 'hosts.search', 'Alt+Shift+KeyN', false)
    expect('error' in outcome && outcome.error).toContain('「新建主机连接」')
  })
  it('saves a free binding as an override', () => {
    expect(applyRecorded({}, 'hosts.search', '$mod+Alt+KeyJ', false)).toEqual({
      overrides: { 'hosts.search': '$mod+Alt+KeyJ' },
    })
  })
  it('rejects Shift-only combinations except function keys', () => {
    expect(combinationProblem('Shift+KeyA', false)).not.toBeNull()
    expect(combinationProblem('Shift+F5', false)).toBeNull()
    expect(combinationProblem('Alt+KeyA', false)).toBeNull()
    expect(combinationProblem('KeyA', false)).not.toBeNull()
  })
})
