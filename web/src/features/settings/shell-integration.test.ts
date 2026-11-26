import { describe, expect, it } from 'vitest'
import { reportedDirectory } from '../../shared/protocol/terminal'
import { osc7SafeCharacters, shellSnippets } from './shell-integration'

/** Reference model of the snippets' encoding: byte-wise percent-encoding outside the safe set. */
function encodeLikeSnippet(path: string): string {
  const safe = new RegExp(`^[${osc7SafeCharacters}]$`)
  let encoded = ''
  for (const byte of new TextEncoder().encode(path)) {
    const character = String.fromCharCode(byte)
    encoded +=
      byte < 0x80 && safe.test(character)
        ? character
        : `%${byte.toString(16).toUpperCase().padStart(2, '0')}`
  }
  return encoded
}

describe('OSC 7 shell snippets', () => {
  it('produce paths that the terminal accepts and decodes exactly', () => {
    for (const path of [
      '/home/dev',
      '/srv/中文 目录',
      '/tmp/100%/#hash?x=1&y',
      '/tmp/it\'s "quoted"',
      '/tmp/new\nline',
      '/tmp/~user/a-b_c.d',
      '/tmp/emoji😀',
    ])
      expect(reportedDirectory(`file://web-01${encodeLikeSnippet(path)}`, 'web-01')).toBe(path)
  })
  it('report through the configured host variable and the OSC 7 sequence', () => {
    for (const snippet of shellSnippets) {
      expect(snippet.code).toContain(String.raw`printf '\e]7;file://%s%s\e\\'`)
      expect(snippet.code).toContain(`"${snippet.hostVariable}" "$encoded"`)
      expect(snippet.code).toContain(`[${osc7SafeCharacters}])`)
      expect(snippet.code).toContain('& 255')
      expect(snippet.code).toContain(snippet.hook)
      expect(snippet.code.endsWith('\n')).toBe(true)
      expect(snippet.code).not.toMatch(/\r|\t/)
    }
  })
  it('keep bash locale-independent and safe to source twice', () => {
    const bash = shellSnippets.find((snippet) => snippet.kind === 'bash')!.code
    expect(bash).toContain('LC_ALL=C')
    expect(bash).toContain('local status=$?')
    expect(bash).toContain('return $status')
    expect(bash).toContain('[[ $PROMPT_COMMAND == *__osc7_cwd* ]] ||')
    const zsh = shellSnippets.find((snippet) => snippet.kind === 'zsh')!.code
    expect(zsh).toContain('setopt no_multibyte')
    expect(zsh).toContain('add-zsh-hook precmd __osc7_cwd')
  })
})
