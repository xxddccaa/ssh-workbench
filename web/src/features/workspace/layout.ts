export type Split = 'single' | 'columns' | 'rows' | 'grid'
export type Slots = (string | null)[]

export const paneCount: Record<Split, number> = { single: 1, columns: 2, rows: 2, grid: 4 }

/**
 * Fits the visible tabs into a new pane count: the focused tab stays
 * visible, other visible tabs keep their order, and empty panes are filled
 * with hidden tabs in tab order.
 */
export function resizeSlots(
  slots: Slots,
  split: Split,
  order: string[],
  focused: string | null,
): Slots {
  const count = paneCount[split]
  const visible = slots.filter((key): key is string => !!key && order.includes(key))
  const kept =
    focused && visible.includes(focused)
      ? [focused, ...visible.filter((key) => key !== focused)]
      : visible
  const chosen = kept.slice(0, count)
  if (count > 1) {
    const ordered = visible.filter((key) => chosen.includes(key))
    chosen.splice(0, chosen.length, ...ordered)
  }
  for (const key of order) {
    if (chosen.length >= count) break
    if (!chosen.includes(key)) chosen.push(key)
  }
  return Array.from({ length: count }, (_, index) => chosen[index] ?? null)
}

/** Shows a tab: focuses its pane if visible, else fills an empty pane or replaces the focused one. */
export function placeTab(
  slots: Slots,
  focusedSlot: number,
  key: string,
): { slots: Slots; focusedSlot: number } {
  const existing = slots.indexOf(key)
  if (existing >= 0) return { slots, focusedSlot: existing }
  const next = [...slots]
  const target = next[focusedSlot] === null ? focusedSlot : next.indexOf(null)
  const slot = target >= 0 ? target : focusedSlot
  next[slot] = key
  return { slots: next, focusedSlot: slot }
}

export function removeTab(slots: Slots, key: string, order: string[]): Slots {
  const remaining = order.filter((candidate) => candidate !== key)
  const next = slots.map((slot) => (slot === key ? null : slot))
  return next.map((slot) => {
    if (slot !== null) return slot
    const hidden = remaining.find((candidate) => !next.includes(candidate))
    if (hidden) next[next.indexOf(null)] = hidden
    return hidden ?? null
  })
}

export function nextIndex(length: number, current: number, step: 1 | -1) {
  if (length === 0) return -1
  return (current + step + length) % length
}
