import { describe, expect, it } from 'vitest'
import { binaryInput, inputFrames, isComposing, outputFrame, reportedDirectory } from './terminal'

describe('terminal byte protocol', () => {
  it('preserves Unicode at every frame boundary', () => {
    const bytes = new TextEncoder().encode('中文😀e\u0301\x1b[31m颜色\x1b[0m')
    for (let split = 1; split < bytes.length; split++) {
      const decoder = new TextDecoder('utf-8', { fatal: true })
      let output = ''
      for (const [offset, data] of [
        [0, bytes.slice(0, split)],
        [split, bytes.slice(split)],
      ] as const) {
        const frame = new ArrayBuffer(9 + data.length)
        const view = new DataView(frame)
        view.setUint8(0, 1)
        view.setBigUint64(1, BigInt(offset))
        new Uint8Array(frame, 9).set(data)
        const parsed = outputFrame(frame)
        expect(parsed.offset).toBe(BigInt(offset))
        output += decoder.decode(parsed.bytes, { stream: true })
      }
      output += decoder.decode()
      expect(output).toBe(new TextDecoder().decode(bytes))
    }
  })
  it('splits input without double encoding or oversized frames', () => {
    const bytes = new TextEncoder().encode('你好😀'.repeat(15000))
    const frames = inputFrames(bytes)
    expect(frames.every((frame) => frame[0] === 2 && frame.length <= 32769)).toBe(true)
    const restored = new Uint8Array(bytes.length)
    let offset = 0
    for (const frame of frames) {
      restored.set(frame.subarray(1), offset)
      offset += frame.length - 1
    }
    expect(restored).toEqual(bytes)
  })
  it('keeps legacy binary input as bytes rather than UTF-8', () => {
    expect([...binaryInput('\x1b\x80\xff')]).toEqual([27, 128, 255])
  })
  it('rejects malformed output', () => {
    expect(() => outputFrame(new ArrayBuffer(8))).toThrow()
    expect(() => outputFrame(new ArrayBuffer(10))).toThrow()
  })
})

describe('terminal safety boundaries', () => {
  it('only accepts directory hints for the current host', () => {
    expect(reportedDirectory('file://example.test/home/%E4%B8%AD%E6%96%87', 'example.test')).toBe(
      '/home/中文',
    )
    for (const value of [
      'file://other.test/etc',
      'https://example.test/etc',
      'file://example.test/tmp/%00x',
      'file:///tmp',
      'file://user@example.test/tmp',
      'file://example.test/tmp?command=bad',
    ])
      expect(reportedDirectory(value, 'example.test')).toBeNull()
  })
  it('does not intercept active IME keyboard events', () => {
    expect(isComposing({ isComposing: true, key: 'Enter', keyCode: 13 } as KeyboardEvent)).toBe(
      true,
    )
    expect(isComposing({ isComposing: false, key: 'Process', keyCode: 0 } as KeyboardEvent)).toBe(
      true,
    )
    expect(isComposing({ isComposing: false, key: 'a', keyCode: 229 } as KeyboardEvent)).toBe(true)
    expect(isComposing({ isComposing: false, key: 'c', keyCode: 67 } as KeyboardEvent)).toBe(false)
  })
})
