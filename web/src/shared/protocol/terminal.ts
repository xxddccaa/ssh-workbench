export const INPUT_LIMIT = 32 * 1024

export function outputFrame(buffer: ArrayBuffer): { offset: bigint; bytes: Uint8Array } {
  if (buffer.byteLength < 10) throw new Error('终端输出帧不完整。')
  const view = new DataView(buffer)
  if (view.getUint8(0) !== 1) throw new Error('不支持的终端输出类型。')
  return { offset: view.getBigUint64(1, false), bytes: new Uint8Array(buffer, 9) }
}

export function inputFrames(bytes: Uint8Array): Uint8Array<ArrayBuffer>[] {
  const frames: Uint8Array<ArrayBuffer>[] = []
  for (let offset = 0; offset < bytes.length; offset += INPUT_LIMIT) {
    const portion = bytes.subarray(offset, offset + INPUT_LIMIT)
    const frame = new Uint8Array(portion.length + 1)
    frame[0] = 2
    frame.set(portion, 1)
    frames.push(frame)
  }
  return frames
}

export function binaryInput(value: string): Uint8Array {
  return Uint8Array.from(value, (character) => character.charCodeAt(0) & 255)
}

export function reportedDirectory(value: string, hostname: string): string | null {
  if (value.length > 8192) return null
  try {
    const location = new URL(value)
    if (
      location.protocol !== 'file:' ||
      location.username ||
      location.password ||
      location.port ||
      location.search ||
      location.hash
    )
      return null
    if (
      location.hostname.replace(/^\[|\]$/g, '').toLowerCase() !==
      hostname.replace(/^\[|\]$/g, '').toLowerCase()
    )
      return null
    const directory = decodeURIComponent(location.pathname)
    if (!directory.startsWith('/') || directory.length > 4096 || directory.includes('\0'))
      return null
    return directory
  } catch {
    return null
  }
}

export function isComposing(event: KeyboardEvent): boolean {
  return event.isComposing || event.key === 'Process' || event.keyCode === 229
}
