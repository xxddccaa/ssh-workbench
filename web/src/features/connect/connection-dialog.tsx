import { useEffect, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowRight,
  FileKey,
  KeyRound,
  LoaderCircle,
  MessageSquareLock,
  ShieldAlert,
  ShieldCheck,
  ShieldX,
} from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type {
  Attempt,
  AuthState,
  Credentials,
  HostProfile,
  LaunchResult,
  Prompt,
  QuickConnectTarget,
} from '../../shared/api/types'
import { Modal } from '../../shared/ui/modal'
import { useNotice } from '../../shared/ui/notices'
import { useAttempt } from './use-attempt'

export type ConnectionPurpose =
  | { kind: 'terminal'; rows: number; cols: number }
  | { kind: 'tunnel'; tunnelID: string; label: string }

function needsInput(host: HostProfile) {
  return host.authMode === 'password' || host.authMode === 'key'
}

function CredentialFields({
  host,
  value,
  onChange,
  onError,
  heading,
  promptMode,
}: {
  host: HostProfile
  value: Credentials
  onChange: (next: Credentials) => void
  onError: (message: string) => void
  heading?: string
  promptMode?: 'password' | 'passphrase'
}) {
  const [filename, setFilename] = useState('')
  const showAutoPassword = host.authMode === 'auto' && promptMode !== 'passphrase'
  const showAutoPassphrase =
    host.authMode === 'auto' && (promptMode === 'passphrase' || !!host.identityFile)
  return (
    <>
      {heading && <h3 className="credential-heading">{heading}</h3>}
      {(host.authMode === 'password' || showAutoPassword) && (
        <label>
          SSH 密码{host.authMode === 'auto' ? '（自动密钥未通过时使用）' : ''}
          <input
            autoFocus={!heading && promptMode !== 'passphrase'}
            type="password"
            autoComplete="off"
            value={value.password ?? ''}
            onChange={(event) => onChange({ ...value, password: event.target.value })}
            placeholder="只用于当前连接"
          />
        </label>
      )}
      {showAutoPassphrase && (
        <label>
          私钥口令{host.identityFile ? `（${host.identityFile}）` : ''}
          <input
            autoFocus={!heading && promptMode === 'passphrase'}
            type="password"
            autoComplete="off"
            value={value.passphrase ?? ''}
            onChange={(event) => onChange({ ...value, passphrase: event.target.value })}
            placeholder="若私钥已加密请填写"
          />
        </label>
      )}
      {host.authMode === 'key' && (
        <>
          {host.identityFile ? (
            <div className="info-box">
              <FileKey size={19} />
              <span>
                使用本机私钥 <code>{host.identityFile}</code>。
              </span>
            </div>
          ) : (
            <label className="key-picker">
              <FileKey size={24} />
              <strong>{filename || '选择私钥文件'}</strong>
              <span>在浏览器中选择，服务端不会读取任意本机路径</span>
              <input
                aria-label={`选择 ${host.name} 的私钥文件`}
                type="file"
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (!file) return
                  if (file.size > 256 * 1024) {
                    onError('私钥文件不得超过 256 KiB。')
                    return
                  }
                  void file
                    .text()
                    .then((privateKey) => {
                      onChange({ ...value, privateKey })
                      setFilename(file.name)
                    })
                    .catch(() => onError('无法读取私钥文件。'))
                }}
              />
            </label>
          )}
          <label>
            私钥口令（若有）
            <input
              type="password"
              autoComplete="off"
              value={value.passphrase ?? ''}
              onChange={(event) => onChange({ ...value, passphrase: event.target.value })}
            />
          </label>
        </>
      )}
      {host.authMode === 'agent' && (
        <div className="info-box">
          <ShieldCheck size={20} />
          <span>使用当前机器的 SSH agent 认证。</span>
        </div>
      )}
      {host.authMode === 'keyboard-interactive' && (
        <div className="info-box">
          <MessageSquareLock size={20} />
          <span>
            交互式认证：连接后会逐条显示服务器的提问（例如密码、验证码），回答只用于本次握手。
          </span>
        </div>
      )}
    </>
  )
}

function knownHostsHint(prompt: Prompt) {
  if (prompt.knownHosts === 'match')
    return (
      <div className="known-hosts match">
        <ShieldCheck size={16} />
        与本机 ~/.ssh/known_hosts 中的记录一致（仅供参考，仍需你确认）。
      </div>
    )
  if (prompt.knownHosts === 'mismatch')
    return (
      <div className="known-hosts mismatch" role="alert">
        <ShieldX size={16} />
        与本机 ~/.ssh/known_hosts 中的记录不一致！除非你确认服务器刚更换过密钥，否则请拒绝。
      </div>
    )
  if (prompt.knownHosts === 'absent')
    return <div className="known-hosts">本机 known_hosts 中没有此主机的记录。</div>
  return null
}

export function ConnectionDialog({
  auth,
  host,
  jumpHost,
  quickConnect,
  initialAttempt,
  initialError = '',
  promptMode,
  purpose,
  onClose,
  onConnected,
}: {
  auth: AuthState
  host: HostProfile
  jumpHost?: HostProfile
  quickConnect?: QuickConnectTarget
  initialAttempt?: Attempt
  initialError?: string
  promptMode?: 'password' | 'passphrase'
  purpose: ConnectionPurpose
  onClose: () => void
  onConnected: (result: LaunchResult) => void
}) {
  const [credentials, setCredentials] = useState<Credentials>({})
  const [jumpCredentials, setJumpCredentials] = useState<Credentials>({})
  const [useSaved, setUseSaved] = useState(!!host.credentialId || !!jumpHost?.credentialId)
  const [remember, setRemember] = useState(false)
  const [error, setError] = useState(initialError)
  const [busy, setBusy] = useState(!!initialAttempt)
  const [verified, setVerified] = useState(false)
  const [answers, setAnswers] = useState<string[]>([])
  const { attempt, start, resume, answer, cancel } = useAttempt()
  const notice = useNotice()
  const vault = useQuery({ queryKey: ['vault'], queryFn: api.vault, staleTime: 5000 })
  const prompt = attempt?.state === 'prompt' ? attempt.prompt : undefined
  const savedAvailable = !!host.credentialId || !!jumpHost?.credentialId
  const vaultReady = !!vault.data?.initialized && !!vault.data.unlocked
  const targetNeedsInput =
    (needsInput(host) || host.authMode === 'auto') && !(useSaved && host.credentialId)
  const jumpNeedsInput = !!jumpHost && needsInput(jumpHost) && !(useSaved && jumpHost.credentialId)
  const isEphemeral = host.id.startsWith('sshcfg:') || host.id.startsWith('quick:')

  useEffect(() => {
    if (!initialAttempt) return
    let active = true
    setBusy(true)
    void resume(initialAttempt)
      .then((final) => {
        if (!active) return
        if (final.state === 'connected' && final.result) {
          if (final.result.warning) notice(final.result.warning)
          onConnected(final.result)
          return
        }
        if (final.state === 'failed') setError(final.error?.message ?? '连接失败。')
      })
      .catch((problem: unknown) => {
        if (active) setError(messageOf(problem))
      })
      .finally(() => {
        if (active) setBusy(false)
      })
    return () => {
      active = false
    }
    // Only run when a live attempt is handed over.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialAttempt?.id])

  useEffect(() => {
    setVerified(false)
    setAnswers(prompt?.questions?.map(() => '') ?? [])
  }, [prompt?.id, prompt?.questions])

  async function connect(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const final = await start({
        purpose: purpose.kind,
        hostId: purpose.kind === 'terminal' && !quickConnect ? host.id : undefined,
        quickConnect: purpose.kind === 'terminal' ? quickConnect : undefined,
        tunnelId: purpose.kind === 'tunnel' ? purpose.tunnelID : undefined,
        rows: purpose.kind === 'terminal' ? purpose.rows : undefined,
        cols: purpose.kind === 'terminal' ? purpose.cols : undefined,
        credentials: targetNeedsInput ? credentials : {},
        jumpCredentials: jumpNeedsInput ? jumpCredentials : undefined,
        useSaved,
        rememberLabel: !isEphemeral && remember && targetNeedsInput ? host.name : undefined,
      })
      if (final.state === 'connected' && final.result) {
        if (final.result.warning) notice(final.result.warning)
        setCredentials({})
        setJumpCredentials({})
        onConnected(final.result)
        return
      }
      if (final.state === 'failed') setError(final.error?.message ?? '连接失败。')
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }

  async function respond(accept: boolean, event?: FormEvent) {
    event?.preventDefault()
    try {
      await answer(accept, prompt?.kind === 'keyboard-interactive' ? answers : undefined)
    } catch (problem) {
      setError(messageOf(problem))
    }
  }

  function close() {
    cancel()
    onClose()
  }

  const title =
    prompt?.kind === 'host-key'
      ? '确认服务器身份'
      : prompt?.kind === 'keyboard-interactive'
        ? '服务器认证提问'
        : purpose.kind === 'tunnel'
          ? `启动隧道 ${purpose.label}`
          : `连接 ${host.name}`
  const hopLabel = (hop: string) => (hop === 'jump' ? '跳板机' : '目标主机')

  return (
    <Modal
      title={title}
      description={`${host.username}@${host.hostname}:${host.port}${jumpHost ? ` · 经由 ${jumpHost.name}` : ''}`}
      onClose={close}
    >
      {prompt?.kind === 'host-key' ? (
        <form onSubmit={(event) => void respond(true, event)}>
          <div className="modal-body form-stack">
            <div className="trust-icon">
              <ShieldAlert size={30} />
            </div>
            <p className="muted-copy">
              {hopLabel(prompt.hop)} <strong>{prompt.host}</strong>{' '}
              尚未被信任。请与服务器管理员或可信控制台上的指纹比较，不能只因它出现在这里就接受。
            </p>
            <div className="fingerprint">
              <span>
                {prompt.algorithm} · SHA256 · {prompt.address}
              </span>
              <code>{prompt.fingerprint}</code>
            </div>
            {knownHostsHint(prompt)}
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={verified}
                onChange={(event) => setVerified(event.target.checked)}
              />
              我已通过可信渠道核对指纹
            </label>
          </div>
          <footer className="modal-footer">
            <button type="button" className="button secondary" onClick={() => void respond(false)}>
              拒绝
            </button>
            <button className="button primary" disabled={!verified}>
              <ShieldCheck size={16} />
              信任并继续
            </button>
          </footer>
        </form>
      ) : prompt?.kind === 'keyboard-interactive' ? (
        <form onSubmit={(event) => void respond(true, event)}>
          <div className="modal-body form-stack">
            <p className="muted-copy">
              {hopLabel(prompt.hop)} <strong>{prompt.host}</strong> 要求回答以下问题。
              {prompt.name && <span className="prompt-name">{prompt.name}</span>}
            </p>
            {prompt.instruction && <pre className="prompt-instruction">{prompt.instruction}</pre>}
            {prompt.questions?.map((question, index) => (
              <label key={`${prompt.id}-${index}`}>
                {question.text || `问题 ${index + 1}`}
                <input
                  autoFocus={index === 0}
                  type={question.echo ? 'text' : 'password'}
                  autoComplete={question.echo ? 'off' : 'one-time-code'}
                  value={answers[index] ?? ''}
                  onChange={(event) =>
                    setAnswers((previous) =>
                      previous.map((item, position) =>
                        position === index ? event.target.value : item,
                      ),
                    )
                  }
                />
              </label>
            ))}
          </div>
          <footer className="modal-footer">
            <button type="button" className="button secondary" onClick={() => void respond(false)}>
              取消认证
            </button>
            <button className="button primary">
              <KeyRound size={16} />
              提交回答
            </button>
          </footer>
        </form>
      ) : (
        <form onSubmit={(event) => void connect(event)}>
          <div className="modal-body form-stack">
            {purpose.kind === 'tunnel' && (
              <p className="muted-copy">隧道使用独立的 SSH 连接，关闭终端或面板不会停止它。</p>
            )}
            {savedAvailable && (
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={useSaved}
                  onChange={(event) => setUseSaved(event.target.checked)}
                />
                使用加密凭据库中保存的凭据
                {!vaultReady && (
                  <span className="field-note">（凭据库未解锁，请先在设置中解锁）</span>
                )}
              </label>
            )}
            {jumpHost && jumpNeedsInput && (
              <CredentialFields
                host={jumpHost}
                value={jumpCredentials}
                onChange={setJumpCredentials}
                onError={setError}
                heading={`跳板机 ${jumpHost.name}`}
              />
            )}
            {jumpHost && (needsInput(host) ? targetNeedsInput : true) && (
              <h3 className="credential-heading">目标主机 {host.name}</h3>
            )}
            {(targetNeedsInput || !needsInput(host)) && (
              <CredentialFields
                host={host}
                value={credentials}
                onChange={setCredentials}
                onError={setError}
                promptMode={promptMode}
              />
            )}
            {!isEphemeral && targetNeedsInput && vault.data?.initialized && (
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={remember}
                  disabled={!vaultReady}
                  onChange={(event) => setRemember(event.target.checked)}
                />
                连接成功后加密保存此凭据并关联到主机
              </label>
            )}
            <p className="form-hint">
              {auth.remote
                ? '凭据只在服务端内存中用于本次会话；只有勾选保存时才会加密写入凭据库。'
                : '凭据只用于本次 SSH/SFTP 会话；只有勾选保存时才会加密写入凭据库。'}
            </p>
            {busy && attempt && (
              <div className="attempt-progress" role="status">
                <LoaderCircle size={15} className="spin" />
                正在建立 SSH 连接、验证主机身份…
              </div>
            )}
            {error && (
              <div className="inline-error" role="alert">
                {error}
              </div>
            )}
          </div>
          <footer className="modal-footer">
            <button type="button" className="button secondary" onClick={close}>
              取消
            </button>
            <button
              className="button primary"
              disabled={
                busy ||
                (targetNeedsInput &&
                  host.authMode === 'key' &&
                  !host.identityFile &&
                  !credentials.privateKey)
              }
            >
              {busy ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}
              {busy ? '正在连接…' : purpose.kind === 'tunnel' ? '启动隧道' : '连接主机'}
            </button>
          </footer>
        </form>
      )}
    </Modal>
  )
}
