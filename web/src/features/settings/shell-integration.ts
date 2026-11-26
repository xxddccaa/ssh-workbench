export type ShellKind = 'bash' | 'zsh'

/**
 * Bytes left unescaped in the reported path. Everything else (spaces, `%`,
 * `#`, `?`, quotes, control characters and every byte of non-ASCII names) is
 * percent-encoded byte by byte, so the URL decodes back to the exact path.
 */
export const osc7SafeCharacters = 'A-Za-z0-9/._~-'

const bash = `__osc7_cwd() {
  local status=$? LC_ALL=C dir=$PWD encoded= char code i
  for (( i = 0; i < \${#dir}; i++ )); do
    char=\${dir:i:1}
    case $char in
      [${osc7SafeCharacters}]) encoded+=$char ;;
      *)
        printf -v code '%d' "'$char"
        printf -v char '%%%02X' $(( code & 255 ))
        encoded+=$char
        ;;
    esac
  done
  printf '\\e]7;file://%s%s\\e\\\\' "$HOSTNAME" "$encoded"
  return $status
}
[[ $PROMPT_COMMAND == *__osc7_cwd* ]] || PROMPT_COMMAND="__osc7_cwd\${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
`

const zsh = `__osc7_cwd() {
  emulate -L zsh
  setopt no_multibyte
  local encoded= char
  for char in "\${(@s::)PWD}"; do
    case $char in
      ([${osc7SafeCharacters}]) encoded+=$char ;;
      (*) encoded+=%\${(l:2::0:)$(( [##16] #char & 255 ))} ;;
    esac
  done
  printf '\\e]7;file://%s%s\\e\\\\' "$HOST" "$encoded"
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd __osc7_cwd
`

export interface ShellSnippet {
  kind: ShellKind
  label: string
  rcFile: string
  hostVariable: string
  hook: string
  code: string
}

export const shellSnippets: ShellSnippet[] = [
  {
    kind: 'bash',
    label: 'Bash',
    rcFile: '~/.bashrc',
    hostVariable: '$HOSTNAME',
    hook: 'PROMPT_COMMAND',
    code: bash,
  },
  {
    kind: 'zsh',
    label: 'Zsh',
    rcFile: '~/.zshrc',
    hostVariable: '$HOST',
    hook: 'precmd',
    code: zsh,
  },
]
