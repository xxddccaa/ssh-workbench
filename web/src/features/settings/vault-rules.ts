export const masterPasswordMinimum = 12
export const resetPhrase = '删除全部凭据'
export const privateKeyLimit = 256 * 1024
const secretByteLimit = 4096

const encoder = new TextEncoder()

export function byteLength(value: string): number {
  return encoder.encode(value).length
}

/** Mirrors the server rule: at least 12 characters, at most 1024 bytes. */
export function masterPasswordProblem(password: string, confirmation: string): string | null {
  if ([...password].length < masterPasswordMinimum)
    return `主密码至少 ${masterPasswordMinimum} 个字符。`
  if (byteLength(password) > 1024) return '主密码不能超过 1024 字节。'
  if (password !== confirmation) return '两次输入的主密码不一致。'
  return null
}

export function credentialLabelProblem(label: string): string | null {
  const trimmed = label.trim()
  if (!trimmed || [...trimmed].length > 80 || /[\0\r\n]/.test(trimmed))
    return '请填写 1–80 个字符的凭据名称，不能包含换行。'
  return null
}

export function secretProblem(value: string, name: string): string | null {
  return byteLength(value) > secretByteLimit ? `${name}不能超过 4096 字节。` : null
}

/** Checks a private key file before it is sent; the server validates it again. */
export function privateKeyProblem(size: number, text?: string): string | null {
  if (size > privateKeyLimit) return '私钥文件不得超过 256 KiB。'
  if (text !== undefined && !text.includes('PRIVATE KEY')) return '所选文件不是受支持的私钥。'
  return null
}

export function resetConfirmed(input: string): boolean {
  return input.trim() === resetPhrase
}
