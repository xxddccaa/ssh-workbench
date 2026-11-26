import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, LoaderCircle, LockKeyhole, ShieldCheck, Terminal } from 'lucide-react'
import { api, messageOf, setCSRF } from '../shared/api/client'
import type { AuthState } from '../shared/api/types'
import { useNotice } from '../shared/ui/notices'
import { Workbench } from './workbench'
import { EditorPanel } from '../features/editor/editor-panel'

const parameters = new URLSearchParams(location.hash.slice(1))
const bootstrap = parameters.get('bootstrap')
if (bootstrap) history.replaceState(null, '', location.pathname + location.search)
let initialAuth: Promise<AuthState> | undefined

function authenticate() {
  if (!initialAuth)
    initialAuth = bootstrap
      ? api.bootstrap(bootstrap).catch(async (error: unknown) => {
          const state = await api.auth()
          if (state.authenticated) return state
          throw error
        })
      : api.auth()
  return initialAuth
}

function initialTheme(): 'dark' | 'light' {
  try {
    return localStorage.getItem('sshwb:theme') === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export function Application() {
  const [theme, setTheme] = useState(initialTheme)
  const queries = useQueryClient()
  const notice = useNotice()
  const authentication = useQuery({
    queryKey: ['auth'],
    queryFn: authenticate,
    retry: false,
    refetchOnWindowFocus: false,
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('sshwb:theme', theme)
    } catch {
      return
    }
  }, [theme])
  useEffect(() => {
    setCSRF(authentication.data?.csrfToken ?? '')
  }, [authentication.data])
  useEffect(() => {
    const expired = () => {
      setCSRF('')
      initialAuth = undefined
      queries.setQueryData<AuthState>(['auth'], (previous) =>
        previous ? { ...previous, authenticated: false, csrfToken: undefined } : undefined,
      )
    }
    const check = () => {
      void api
        .auth()
        .then((state) => {
          if (state.authRequired !== false && !state.authenticated) expired()
        })
        .catch(() => undefined)
    }
    window.addEventListener('workbench:unauthenticated', expired)
    const timer = window.setInterval(check, 30000)
    return () => {
      window.removeEventListener('workbench:unauthenticated', expired)
      clearInterval(timer)
    }
  }, [queries])
  async function logout() {
    try {
      await api.logout()
      setCSRF('')
      initialAuth = undefined
      queries.setQueryData<AuthState>(['auth'], (previous) =>
        previous ? { ...previous, authenticated: false, csrfToken: undefined } : undefined,
      )
      queries.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth' })
    } catch (error) {
      notice(messageOf(error))
    }
  }
  if (authentication.isPending)
    return (
      <div className="startup">
        <span className="brand-mark large">
          <Terminal size={26} />
        </span>
        <LoaderCircle size={20} className="spin" />
        <p>正在验证工作区授权…</p>
      </div>
    )
  if (!authentication.data?.authenticated)
    return (
      <AuthGate
        remote={authentication.data?.remote ?? location.protocol === 'https:'}
        initialError={authentication.error ? messageOf(authentication.error) : ''}
        onAuthenticated={(state) => {
          setCSRF(state.csrfToken ?? '')
          queries.setQueryData(['auth'], state)
          initialAuth = Promise.resolve(state)
        }}
      />
    )
  if (location.pathname === '/editor') {
    const search = new URLSearchParams(location.search)
    const terminalID = search.get('terminal')
    const path = search.get('path')
    if (!terminalID || !path)
      return <div className="startup">编辑器链接不完整，请从 SFTP 文件面板打开。</div>
    return <EditorPanel source={{ kind: 'remote', terminalID, path }} theme={theme} standalone />
  }
  if (location.pathname === '/local-editor') {
    const documentID = new URLSearchParams(location.search).get('document')
    if (!documentID)
      return (
        <div className="startup">本机文件链接不完整，请在终端运行 sshw edit 文件路径。</div>
      )
    return <EditorPanel source={{ kind: 'local', documentID }} theme={theme} standalone />
  }
  return (
    <Workbench
      auth={authentication.data}
      theme={theme}
      toggleTheme={() => setTheme((previous) => (previous === 'dark' ? 'light' : 'dark'))}
      onLogout={() => void logout()}
    />
  )
}

function AuthGate({
  remote,
  initialError,
  onAuthenticated,
}: {
  remote: boolean
  initialError: string
  onAuthenticated: (state: AuthState) => void
}) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState(initialError)
  const [busy, setBusy] = useState(false)
  async function login(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const state = remote ? await api.login(password) : await api.auth()
      if (!state.authenticated) {
        setError('请在终端运行 sshw open 获取新的一次性授权链接。')
        return
      }
      setPassword('')
      onAuthenticated(state)
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="auth-page">
      <div className="auth-card">
        <span className="brand-mark large">
          <Terminal size={29} />
        </span>
        <span className="eyebrow">SSH WORKBENCH</span>
        <h1>{remote ? '登录你的工作区' : '工作区已锁定'}</h1>
        <p>
          {remote
            ? '单用户、受保护的远程访问。'
            : '本机服务也需要授权，不向其他网页开放 SSH 能力。'}
        </p>
        <form className="form-stack" onSubmit={(event) => void login(event)}>
          {remote ? (
            <label>
              所有者密码
              <input
                autoFocus
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </label>
          ) : (
            <div className="info-box">
              <LockKeyhole size={21} />
              <span>
                使用启动时打印的一次性授权链接；链接过期或已使用时，在终端运行{' '}
                <code>sshw open</code> 获取新链接，无需重启服务。
              </span>
            </div>
          )}
          {error && (
            <div className="inline-error" role="alert">
              {error}
            </div>
          )}
          <button className="button primary" disabled={busy}>
            {busy ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}
            {remote ? '解锁工作区' : '重新检查授权'}
          </button>
        </form>
        <small>
          <ShieldCheck size={13} />
          {remote ? 'HTTPS · HttpOnly 会话 · CSRF 防护' : '127.0.0.1 · 一次性授权 · 精确来源检查'}
        </small>
      </div>
    </div>
  )
}
