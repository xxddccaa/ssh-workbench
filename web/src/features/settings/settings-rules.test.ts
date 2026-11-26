import { describe, expect, it } from 'vitest'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import {
  clampInteger,
  fontSizeRange,
  isImeEnter,
  parseInRange,
  scrollbackRange,
} from './preferences'
import {
  credentialLabelProblem,
  masterPasswordProblem,
  privateKeyProblem,
  resetConfirmed,
  secretProblem,
} from './vault-rules'

function keyEvent(key: string, isComposing: boolean, keyCode: number) {
  return { key, nativeEvent: { isComposing, keyCode } } as unknown as ReactKeyboardEvent
}

describe('appearance preferences', () => {
  it('clamps to the schema ranges', () => {
    expect(clampInteger(9, fontSizeRange)).toBe(10)
    expect(clampInteger(29, fontSizeRange)).toBe(28)
    expect(clampInteger(14.6, fontSizeRange)).toBe(15)
    expect(clampInteger(Number.NaN, scrollbackRange)).toBe(500)
  })
  it('accepts only whole numbers inside the range while typing', () => {
    expect(parseInRange('5', scrollbackRange)).toBeNull()
    expect(parseInRange('5000', scrollbackRange)).toBe(5000)
    expect(parseInRange(' 50000 ', scrollbackRange)).toBe(50000)
    expect(parseInRange('50001', scrollbackRange)).toBeNull()
    expect(parseInRange('1e4', scrollbackRange)).toBeNull()
    expect(parseInRange('', scrollbackRange)).toBeNull()
  })
  it('recognises the Enter that confirms an IME candidate', () => {
    expect(isImeEnter(keyEvent('Enter', true, 13))).toBe(true)
    expect(isImeEnter(keyEvent('Enter', false, 229))).toBe(true)
    expect(isImeEnter(keyEvent('Enter', false, 13))).toBe(false)
    expect(isImeEnter(keyEvent('a', true, 229))).toBe(false)
  })
})

describe('vault form rules', () => {
  it('requires a 12 character master password typed twice', () => {
    expect(masterPasswordProblem('short', 'short')).toContain('12')
    expect(masterPasswordProblem('correct horse', 'correct horsf')).toContain('不一致')
    expect(masterPasswordProblem('correct horse', 'correct horse')).toBeNull()
    expect(masterPasswordProblem('中文主密码一二三四五六七', '中文主密码一二三四五六七')).toBeNull()
    expect(masterPasswordProblem('x'.repeat(1025), 'x'.repeat(1025))).toContain('1024')
  })
  it('validates labels like the server', () => {
    expect(credentialLabelProblem('  ')).not.toBeNull()
    expect(credentialLabelProblem('a\nb')).not.toBeNull()
    expect(credentialLabelProblem('生产密钥')).toBeNull()
    expect(credentialLabelProblem('键'.repeat(80))).toBeNull()
    expect(credentialLabelProblem('键'.repeat(81))).not.toBeNull()
  })
  it('limits secrets and private keys', () => {
    expect(secretProblem('x'.repeat(4096), '密码')).toBeNull()
    expect(secretProblem('é'.repeat(2049), '密码')).not.toBeNull()
    expect(privateKeyProblem(256 * 1024 + 1)).toContain('256 KiB')
    expect(privateKeyProblem(10, 'ssh-ed25519 AAAA… public')).not.toBeNull()
    expect(privateKeyProblem(10, '-----BEGIN OPENSSH PRIVATE KEY-----')).toBeNull()
  })
  it('requires the exact reset phrase', () => {
    expect(resetConfirmed('删除全部凭据')).toBe(true)
    expect(resetConfirmed(' 删除全部凭据 ')).toBe(true)
    expect(resetConfirmed('删除凭据')).toBe(false)
    expect(resetConfirmed('DELETE ALL SAVED CREDENTIALS')).toBe(false)
  })
})
