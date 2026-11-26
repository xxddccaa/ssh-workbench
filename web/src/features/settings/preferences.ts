import type { KeyboardEvent as ReactKeyboardEvent } from 'react'

export const fontSizeRange = { min: 10, max: 28 } as const
export const scrollbackRange = { min: 500, max: 50000, step: 500 } as const

export function clampInteger(value: number, range: { min: number; max: number }): number {
  if (!Number.isFinite(value)) return range.min
  return Math.min(range.max, Math.max(range.min, Math.round(value)))
}

/** Parses a typed integer; returns null unless it is a whole number inside the range. */
export function parseInRange(text: string, range: { min: number; max: number }): number | null {
  const trimmed = text.trim()
  if (!/^\d{1,9}$/.test(trimmed)) return null
  const value = Number(trimmed)
  return value >= range.min && value <= range.max ? value : null
}

/**
 * True for the Enter that confirms an IME candidate. Browsers report it with
 * isComposing or keyCode 229; it must not submit a form.
 */
export function isImeEnter(event: ReactKeyboardEvent): boolean {
  return (
    event.key === 'Enter' && (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229)
  )
}

/** onKeyDown handler for text inputs inside forms: blocks IME-confirming Enter from submitting. */
export function guardImeEnter(event: ReactKeyboardEvent) {
  if (isImeEnter(event)) event.preventDefault()
}
