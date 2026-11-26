import { useId, useState, type ChangeEvent, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FileKey,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  LockKeyholeOpen,
  Plus,
  RotateCcw,
  ShieldAlert,
  Trash2,
} from 'lucide-react'
import { ApiError, api, messageOf } from '../../shared/api/client'
import type { AuthState, CredentialRecord, VaultStatus } from '../../shared/api/types'
import { formatBytes } from '../../shared/lib/format'
import { useNotice } from '../../shared/ui/notices'
import { guardImeEnter } from './preferences'
import { SettingsHeading } from './settings-heading'
import {
  credentialLabelProblem,
  masterPasswordMinimum,
  masterPasswordProblem,
  privateKeyProblem,
  resetConfirmed,
  resetPhrase,
  secretProblem,
} from './vault-rules'

function codeOf(error: unknown): string {
  return error instanceof ApiError ? error.code : ''
}

function fullDate(value: string): string {
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

/**
 * Runs one request with local busy/error state. Secrets are passed through
 * closures only, never kept in a query or mutation cache.
 */
function useAction() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [code, setCode] = useState('')
  async function run<Result>(task: () => Promise<Result>): Promise<Result | undefined> {
    setBusy(true)
    setError('')
    setCode('')
    try {
      return await task()
    } catch (problem) {
      setError(messageOf(problem))
      setCode(codeOf(problem))
      return undefined
    } finally {
      setBusy(false)
    }
  }
  function fail(message: string) {
    setError(message)
    setCode('')
  }
  function clear() {
    setError('')
    setCode('')
  }
  return { busy, error, code, run, fail, clear }
}

function useVaultChanged() {
  const queries = useQueryClient()
  return async (status: VaultStatus | undefined, alsoHosts = false) => {
    if (status) queries.setQueryData(['vault'], status)
    await Promise.all([
      queries.invalidateQueries({ queryKey: ['vault'] }),
      queries.invalidateQueries({ queryKey: ['credentials'] }),
      alsoHosts ? queries.invalidateQueries({ queryKey: ['hosts'] }) : undefined,
    ])
  }
}

function ErrorLine({ message }: { message: string }) {
  if (!message) return null
  return (
    <div className="inline-error" role="alert">
      {message}
    </div>
  )
}

export function VaultSection({ auth }: { auth: AuthState }) {
  const vault = useQuery({ queryKey: ['vault'], queryFn: api.vault })
  return (
    <>
      <SettingsHeading title="凭据库">
        保存 SSH 密码或私钥是可选的。不启用时，凭据只在连接时使用，并且只保留在当前会话内存中。
      </SettingsHeading>
      {vault.isPending ? (
        <p className="muted-copy settings-loading">
          <LoaderCircle className="spin" size={16} />
          正在读取凭据库状态…
        </p>
      ) : vault.isError ? (
        <>
          <ErrorLine message={messageOf(vault.error)} />
          <div>
            <button
              type="button"
              className="button secondary small"
              onClick={() => void vault.refetch()}
            >
              <RotateCcw size={14} />
              重试
            </button>
          </div>
          {codeOf(vault.error) === 'vault_corrupted' && (
            <ResetPanel intro="凭据库配置无法识别时，只能重置后重新启用。" />
          )}
        </>
      ) : (
        <>
          <VaultState status={vault.data} />
          {!vault.data.initialized ? (
            <SetupPanel auth={auth} />
          ) : !vault.data.unlocked ? (
            <LockedPanel status={vault.data} />
          ) : (
            <>
              <CredentialList />
              <LockPanel status={vault.data} />
              <ResetPanel />
            </>
          )}
        </>
      )}
    </>
  )
}

function VaultState({ status }: { status: VaultStatus }) {
  const state = !status.initialized ? '未启用' : status.unlocked ? '已解锁' : '已锁定'
  const mode =
    status.mode === 'keyring' ? '系统凭据存储保护' : status.mode === 'password' ? '主密码保护' : ''
  return (
    <p className="settings-vault-state" aria-live="polite">
      <span
        className={`status-dot ${status.unlocked ? 'online' : status.initialized ? 'waiting' : ''}`}
      />
      状态：{state}
      {mode && ` · ${mode}`}
    </p>
  )
}

function SetupPanel({ auth }: { auth: AuthState }) {
  const [mode, setMode] = useState<'keyring' | 'password'>('keyring')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [attempted, setAttempted] = useState(false)
  const action = useAction()
  const changed = useVaultChanged()
  const notify = useNotice()
  const problem = mode === 'password' ? masterPasswordProblem(password, confirmation) : null

  async function submit(event: FormEvent) {
    event.preventDefault()
    setAttempted(true)
    if (problem || action.busy) return
    const status = await action.run(() =>
      api.setupVault(mode, mode === 'password' ? password : undefined),
    )
    if (!status) return
    setPassword('')
    setConfirmation('')
    notify('凭据库已启用。')
    await changed(status)
  }

  return (
    <form className="settings-block form-stack" onSubmit={(event) => void submit(event)}>
      <div className="setting-detail">
        <KeyRound size={20} />
        <div>
          <strong>启用后如何保存</strong>
          <p>
            每条凭据都用一把随机生成的数据密钥加密后，保存在服务所在机器的本机数据库中；数据密钥本身由下面选择的方式保护。保存后的密码和私钥不会再显示到浏览器。
          </p>
        </div>
      </div>
      <fieldset className="settings-fieldset">
        <legend>数据密钥的保护方式</legend>
        <div className="settings-options">
          <label
            className={`checkbox-label settings-option ${mode === 'keyring' ? 'selected' : ''}`}
          >
            <input
              type="radio"
              name="vault-mode"
              checked={mode === 'keyring'}
              onChange={() => {
                setMode('keyring')
                action.clear()
              }}
            />
            <span>
              <strong>系统凭据存储</strong>
              <small>
                macOS 钥匙串 / Windows 凭据管理器 / Linux Secret
                Service。服务可自动读取密钥并解锁，无需额外记住密码。
                {auth.remote && ' 远程模式下，这指运行 SSH Workbench 服务的那台机器。'}
              </small>
            </span>
          </label>
          <label
            className={`checkbox-label settings-option ${mode === 'password' ? 'selected' : ''}`}
          >
            <input
              type="radio"
              name="vault-mode"
              checked={mode === 'password'}
              onChange={() => {
                setMode('password')
                action.clear()
              }}
            />
            <span>
              <strong>主密码</strong>
              <small>
                至少 {masterPasswordMinimum}{' '}
                个字符，每次服务启动后需输入一次。主密码无法找回，忘记后只能重置凭据库，已保存的凭据会全部丢失。
              </small>
            </span>
          </label>
        </div>
      </fieldset>
      {mode === 'password' && (
        <div className="form-row">
          <label>
            主密码
            <input
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>
          <label>
            再次输入主密码
            <input
              type="password"
              autoComplete="new-password"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </label>
        </div>
      )}
      {attempted && problem && <ErrorLine message={problem} />}
      {action.error && (
        <div className="inline-error" role="alert">
          {action.error}
          {action.code === 'keyring_unavailable' && mode === 'keyring' && (
            <>
              {' '}
              <button
                type="button"
                className="settings-inline-link"
                onClick={() => {
                  setMode('password')
                  action.clear()
                }}
              >
                改用主密码
              </button>
            </>
          )}
        </div>
      )}
      <div className="settings-actions">
        <button type="submit" className="button primary" disabled={action.busy}>
          {action.busy ? <LoaderCircle className="spin" size={15} /> : <LockKeyhole size={15} />}
          启用凭据库
        </button>
      </div>
    </form>
  )
}

function LockedPanel({ status }: { status: VaultStatus }) {
  const [password, setPassword] = useState('')
  const action = useAction()
  const changed = useVaultChanged()

  async function unlock(event?: FormEvent) {
    event?.preventDefault()
    if (action.busy) return
    const typed = password
    setPassword('')
    const result = await action.run(() =>
      api.unlockVault(status.mode === 'password' ? typed : undefined),
    )
    if (result) await changed(result)
  }

  if (status.mode !== 'password')
    return (
      <>
        <div className="setting-detail">
          <ShieldAlert size={20} />
          <div>
            <strong>系统凭据存储暂时不可用</strong>
            <p>
              无法从系统凭据存储读取凭据库密钥，保存的凭据暂时不能使用。请确认服务所在机器的钥匙串或
              Secret Service 已解锁（例如已登录图形会话），然后重试。
            </p>
          </div>
        </div>
        <ErrorLine message={action.error} />
        <div className="settings-actions">
          <button
            type="button"
            className="button secondary"
            disabled={action.busy}
            onClick={() => void unlock()}
          >
            {action.busy ? <LoaderCircle className="spin" size={15} /> : <RotateCcw size={15} />}
            重试
          </button>
        </div>
        <ResetPanel />
      </>
    )

  return (
    <>
      <form className="settings-block form-stack" onSubmit={(event) => void unlock(event)}>
        <p className="muted-copy">
          凭据库已锁定。输入主密码解锁后，才能使用或管理已保存的凭据。服务重启后需要重新解锁。
        </p>
        <label>
          主密码
          <input
            type="password"
            autoComplete="current-password"
            autoFocus
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        <ErrorLine message={action.error} />
        <div className="settings-actions">
          <button type="submit" className="button primary" disabled={!password || action.busy}>
            {action.busy ? (
              <LoaderCircle className="spin" size={15} />
            ) : (
              <LockKeyholeOpen size={15} />
            )}
            解锁
          </button>
        </div>
      </form>
      <ResetPanel intro="忘记主密码时，重置是唯一的办法。" />
    </>
  )
}

function CredentialList() {
  const credentials = useQuery({ queryKey: ['credentials'], queryFn: api.credentials })
  const [adding, setAdding] = useState(false)
  const action = useAction()
  const queries = useQueryClient()
  const notify = useNotice()

  async function remove(record: CredentialRecord) {
    if (
      !window.confirm(
        `删除凭据“${record.label}”？\n\n使用它的主机会解除关联，之后连接时需要重新输入密码或选择私钥。此操作无法撤销。`,
      )
    )
      return
    const done = await action.run(async () => {
      await api.deleteCredential(record.id)
      return true
    })
    if (!done) return
    notify(`已删除凭据“${record.label}”。`)
    queries.setQueryData<CredentialRecord[]>(['credentials'], (list) =>
      list?.filter((item) => item.id !== record.id),
    )
    await Promise.all([
      queries.invalidateQueries({ queryKey: ['credentials'] }),
      queries.invalidateQueries({ queryKey: ['hosts'] }),
    ])
  }

  return (
    <section className="settings-block form-stack" aria-labelledby="vault-credentials-title">
      <div className="settings-block-header">
        <h4 id="vault-credentials-title">已保存的凭据</h4>
        {!adding && (
          <button type="button" className="button secondary small" onClick={() => setAdding(true)}>
            <Plus size={14} />
            添加凭据
          </button>
        )}
      </div>
      {adding && <AddCredentialForm onDone={() => setAdding(false)} />}
      {credentials.isPending ? (
        <p className="muted-copy settings-loading">
          <LoaderCircle className="spin" size={16} />
          正在读取…
        </p>
      ) : credentials.isError ? (
        <ErrorLine message={messageOf(credentials.error)} />
      ) : credentials.data.length === 0 ? (
        <p className="muted-copy">还没有保存的凭据。</p>
      ) : (
        <ul className="settings-credentials">
          {credentials.data.map((record) => (
            <li key={record.id}>
              {record.kind === 'key' ? <FileKey size={17} /> : <KeyRound size={17} />}
              <div>
                <strong>{record.label}</strong>
                <span>
                  {record.kind === 'key' ? '私钥' : '密码'} · 保存于 {fullDate(record.createdAt)}
                </span>
              </div>
              <button
                type="button"
                className="icon-button danger-hover"
                aria-label={`删除凭据 ${record.label}`}
                title="删除凭据"
                disabled={action.busy}
                onClick={() => void remove(record)}
              >
                <Trash2 size={15} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <ErrorLine message={action.error} />
      <p className="form-hint">
        在“编辑主机 →
        保存的凭据”中关联后，连接时会自动使用；也可以在连接对话框中勾选“连接成功后加密保存此凭据并关联到主机”。
      </p>
    </section>
  )
}

interface SelectedKey {
  name: string
  size: number
  text: string
}

function AddCredentialForm({ onDone }: { onDone: () => void }) {
  const id = useId()
  const [label, setLabel] = useState('')
  const [kind, setKind] = useState<'password' | 'key'>('password')
  const [password, setPassword] = useState('')
  const [key, setKey] = useState<SelectedKey | null>(null)
  const [passphrase, setPassphrase] = useState('')
  const action = useAction()
  const queries = useQueryClient()
  const notify = useNotice()

  function forgetSecrets() {
    setPassword('')
    setKey(null)
    setPassphrase('')
  }

  async function pick(event: ChangeEvent<HTMLInputElement>) {
    const input = event.currentTarget
    const file = input.files?.[0]
    input.value = ''
    if (!file) return
    setKey(null)
    const sizeProblem = privateKeyProblem(file.size)
    if (sizeProblem) {
      action.fail(sizeProblem)
      return
    }
    try {
      const text = await file.text()
      const problem = privateKeyProblem(file.size, text)
      if (problem) {
        action.fail(problem)
        return
      }
      action.clear()
      setKey({ name: file.name, size: file.size, text })
    } catch {
      action.fail('无法读取私钥文件。')
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (action.busy) return
    const problem =
      credentialLabelProblem(label) ??
      (kind === 'password'
        ? password
          ? secretProblem(password, '密码')
          : '请填写密码。'
        : key
          ? (privateKeyProblem(key.size, key.text) ?? secretProblem(passphrase, '私钥口令'))
          : '请选择私钥文件。')
    if (problem) {
      action.fail(problem)
      return
    }
    const name = label.trim()
    const secret =
      kind === 'password'
        ? { password }
        : { privateKey: key?.text ?? '', passphrase: passphrase || undefined }
    const record = await action.run(() => api.createCredential(name, secret))
    if (!record) return
    forgetSecrets()
    setLabel('')
    notify(`已保存凭据“${record.label}”。`)
    queries.setQueryData<CredentialRecord[]>(['credentials'], (list) => [...(list ?? []), record])
    onDone()
    await queries.invalidateQueries({ queryKey: ['credentials'] })
  }

  return (
    <form
      className="settings-subform form-stack"
      aria-label="添加凭据"
      onSubmit={(event) => void submit(event)}
    >
      <label>
        名称
        <input
          autoFocus
          maxLength={80}
          placeholder="例如：生产环境部署密钥"
          value={label}
          onChange={(event) => setLabel(event.target.value)}
          onKeyDown={guardImeEnter}
        />
      </label>
      <fieldset className="settings-fieldset">
        <legend>类型</legend>
        <div className="choice-row">
          {(
            [
              ['password', '密码'],
              ['key', '私钥'],
            ] as const
          ).map(([value, text]) => (
            <button
              key={value}
              type="button"
              className={`choice ${kind === value ? 'selected' : ''}`}
              aria-pressed={kind === value}
              onClick={() => {
                setKind(value)
                forgetSecrets()
                action.clear()
              }}
            >
              {text}
            </button>
          ))}
        </div>
      </fieldset>
      {kind === 'password' ? (
        <label>
          SSH 密码
          <input
            type="password"
            autoComplete="off"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
      ) : (
        <>
          <label className="key-picker" htmlFor={`${id}-key`}>
            <FileKey size={24} />
            <strong>{key ? key.name : '选择私钥文件'}</strong>
            <span>
              {key
                ? `${formatBytes(key.size)} · 已读取，内容不会显示`
                : '在浏览器中读取，最大 256 KiB；服务端不会读取任意本机路径'}
            </span>
            <input id={`${id}-key`} type="file" onChange={(event) => void pick(event)} />
          </label>
          <label>
            私钥口令（若有）
            <input
              type="password"
              autoComplete="off"
              value={passphrase}
              onChange={(event) => setPassphrase(event.target.value)}
            />
          </label>
        </>
      )}
      <ErrorLine message={action.error} />
      <div className="settings-actions">
        <button
          type="button"
          className="button secondary"
          disabled={action.busy}
          onClick={() => {
            forgetSecrets()
            onDone()
          }}
        >
          取消
        </button>
        <button type="submit" className="button primary" disabled={action.busy}>
          {action.busy && <LoaderCircle className="spin" size={15} />}
          加密保存
        </button>
      </div>
    </form>
  )
}

function LockPanel({ status }: { status: VaultStatus }) {
  const action = useAction()
  const changed = useVaultChanged()
  if (status.mode !== 'password')
    return (
      <div className="setting-detail">
        <LockKeyholeOpen size={20} />
        <div>
          <strong>由系统凭据存储自动解锁</strong>
          <p>
            服务会在需要时从系统凭据存储读取密钥，手动锁定会立即被重新解锁，因此这里不提供锁定。若希望每次手动解锁，可重置凭据库后改用主密码（已保存的凭据会全部删除）。
          </p>
        </div>
      </div>
    )
  return (
    <section className="settings-block form-stack" aria-labelledby="vault-lock-title">
      <div className="settings-field">
        <div>
          <strong id="vault-lock-title">锁定凭据库</strong>
          <span>锁定后，保存的凭据在重新输入主密码前不可用。服务重启后也会自动锁定。</span>
        </div>
        <button
          type="button"
          className="button secondary small"
          disabled={action.busy}
          onClick={() =>
            void action.run(api.lockVault).then((result) => {
              if (result) return changed(result)
            })
          }
        >
          <LockKeyhole size={14} />
          立即锁定
        </button>
      </div>
      <ErrorLine message={action.error} />
    </section>
  )
}

function ResetPanel({ intro }: { intro?: string }) {
  const [open, setOpen] = useState(false)
  const [phrase, setPhrase] = useState('')
  const action = useAction()
  const changed = useVaultChanged()
  const notify = useNotice()
  const id = useId()

  async function reset() {
    if (!resetConfirmed(phrase) || action.busy) return
    const status = await action.run(api.resetVault)
    if (!status) return
    setPhrase('')
    setOpen(false)
    notify('凭据库已重置，所有已保存的凭据均已删除。')
    await changed(status, true)
  }

  return (
    <section className="settings-block settings-danger form-stack" aria-labelledby={`${id}-title`}>
      <div className="settings-field">
        <div>
          <strong id={`${id}-title`}>重置凭据库</strong>
          <span>{intro ?? '删除全部已保存的凭据并停用凭据库。'}</span>
        </div>
        {!open && (
          <button type="button" className="button danger small" onClick={() => setOpen(true)}>
            重置…
          </button>
        )}
      </div>
      {open && (
        <>
          <p className="settings-danger-copy">
            重置会销毁凭据库密钥并删除每一条已保存的凭据，它们将永久无法读取，也无法恢复。关联这些凭据的主机会解除关联，主机配置本身保留。之后可以重新启用凭据库。
          </p>
          <label>
            输入“{resetPhrase}”以确认
            <input
              autoComplete="off"
              spellCheck={false}
              value={phrase}
              onChange={(event) => setPhrase(event.target.value)}
            />
          </label>
          <ErrorLine message={action.error} />
          <div className="settings-actions">
            <button
              type="button"
              className="button secondary"
              disabled={action.busy}
              onClick={() => {
                setOpen(false)
                setPhrase('')
                action.clear()
              }}
            >
              取消
            </button>
            <button
              type="button"
              className="button danger"
              disabled={!resetConfirmed(phrase) || action.busy}
              onClick={() => void reset()}
            >
              {action.busy ? <LoaderCircle className="spin" size={15} /> : <Trash2 size={15} />}
              永久删除全部凭据
            </button>
          </div>
        </>
      )}
    </section>
  )
}
