import { describe, expect, it } from 'vitest'
import { nextIndex, placeTab, removeTab, resizeSlots } from './layout'

describe('pane layout', () => {
  it('fills new panes from hidden tabs and keeps the focused tab when shrinking', () => {
    expect(resizeSlots(['b'], 'grid', ['a', 'b', 'c'], 'b')).toEqual(['b', 'a', 'c', null])
    expect(resizeSlots(['a', 'b', 'c', null], 'single', ['a', 'b', 'c'], 'c')).toEqual(['c'])
    expect(resizeSlots(['a', 'b', 'c', 'd'], 'columns', ['a', 'b', 'c', 'd'], 'c')).toEqual([
      'a',
      'c',
    ])
    expect(resizeSlots([null], 'rows', [], null)).toEqual([null, null])
  })

  it('focuses visible tabs and prefers empty panes for new ones', () => {
    expect(placeTab(['a', 'b'], 0, 'b')).toEqual({ slots: ['a', 'b'], focusedSlot: 1 })
    expect(placeTab(['a', null], 0, 'c')).toEqual({ slots: ['a', 'c'], focusedSlot: 1 })
    expect(placeTab(['a', 'b'], 1, 'c')).toEqual({ slots: ['a', 'c'], focusedSlot: 1 })
  })

  it('backfills a closed pane with a hidden tab', () => {
    expect(removeTab(['a', 'b'], 'a', ['a', 'b', 'c'])).toEqual(['c', 'b'])
    expect(removeTab(['a', 'b'], 'b', ['a', 'b'])).toEqual(['a', null])
  })

  it('wraps tab navigation', () => {
    expect(nextIndex(3, 2, 1)).toBe(0)
    expect(nextIndex(3, 0, -1)).toBe(2)
    expect(nextIndex(0, 0, 1)).toBe(-1)
  })
})
