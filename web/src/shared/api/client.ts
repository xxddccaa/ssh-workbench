import type {
  Attempt,
  AttemptRequest,
  AuthState,
  CredentialRecord,
  Credentials,
  FileListing,
  HostInput,
  HostProfile,
  ImportPreview,
  ImportResult,
  LocalDocument,
  Mutation,
  RemoteDocument,
  SSHConfigHosts,
  Snippet,
  SnippetInput,
  TerminalInfo,
  TreeUsage,
  Tunnel,
  TunnelInput,
  VaultStatus,
  Workspace,
} from './types'

let csrfToken = ''

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function setCSRF(token: string) {
  csrfToken = token
}

export function messageOf(error: unknown) {
  return error instanceof Error ? error.message : '操作未完成，请重试。'
}

export function apiError(payload: unknown, fallback = '服务暂时不可用，请检查连接。'): ApiError {
  if (typeof payload === 'object' && payload !== null && 'error' in payload) {
    const problem = payload.error
    if (
      typeof problem === 'object' &&
      problem !== null &&
      'code' in problem &&
      'message' in problem
    ) {
      return new ApiError(
        String(problem.code),
        String(problem.message),
        'details' in problem ? problem.details : undefined,
      )
    }
  }
  return new ApiError('network_error', fallback)
}

async function request<Result>(route: string, options: RequestInit = {}): Promise<Result> {
  const headers = new Headers(options.headers)
  if (options.body) headers.set('Content-Type', 'application/json')
  if (options.method && options.method !== 'GET' && csrfToken)
    headers.set('X-CSRF-Token', csrfToken)
  const response = await fetch(`/api/v1${route}`, {
    ...options,
    headers,
    credentials: 'same-origin',
  })
  if (!response.ok) {
    const problem = apiError(await response.json().catch(() => null))
    if (problem.code === 'unauthenticated')
      window.dispatchEvent(new Event('workbench:unauthenticated'))
    throw problem
  }
  if (response.status === 204 || response.headers.get('Content-Length') === '0')
    return undefined as Result
  return response.json() as Promise<Result>
}

const encoded = encodeURIComponent

const json = (method: string, value?: unknown): RequestInit => ({
  method,
  body: value === undefined ? undefined : JSON.stringify(value),
})

export const api = {
  auth: () => request<AuthState>('/auth'),
  bootstrap: (token: string) => request<AuthState>('/auth/bootstrap', json('POST', { token })),
  login: (password: string) => request<AuthState>('/auth/login', json('POST', { password })),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  hosts: () => request<HostProfile[]>('/hosts'),
  saveHost: (host: HostInput, hostID?: string) => {
    const {
      name,
      hostname,
      port,
      username,
      authMode,
      group,
      color,
      favorite,
      production,
      jumpHostId,
      proxyCommand,
      identityFile,
      credentialId,
    } = host
    return request<HostProfile>(
      `/hosts${hostID ? `/${encoded(hostID)}` : ''}`,
      json(hostID ? 'PUT' : 'POST', {
        name,
        hostname,
        port,
        username,
        authMode,
        group,
        color,
        favorite,
        production: production ?? false,
        jumpHostId: jumpHostId || undefined,
        proxyCommand: proxyCommand || undefined,
        identityFile: identityFile || undefined,
        credentialId: credentialId || undefined,
      }),
    )
  },
  deleteHost: (hostID: string) => request<void>(`/hosts/${encoded(hostID)}`, { method: 'DELETE' }),
  startAttempt: (input: AttemptRequest) => request<Attempt>('/attempts', json('POST', input)),
  waitAttempt: (attemptID: string, after: number, signal?: AbortSignal) =>
    request<Attempt>(`/attempts/${encoded(attemptID)}?after=${after}`, { signal }),
  answerAttempt: (attemptID: string, promptId: string, accept: boolean, responses?: string[]) =>
    request<void>(
      `/attempts/${encoded(attemptID)}/answer`,
      json('POST', { promptId, accept, responses }),
    ),
  cancelAttempt: (attemptID: string) =>
    request<void>(`/attempts/${encoded(attemptID)}`, { method: 'DELETE' }),
  terminals: () => request<TerminalInfo[]>('/terminals'),
  closeTerminal: (terminalID: string) =>
    request<void>(`/terminals/${encoded(terminalID)}`, { method: 'DELETE' }),
  files: (terminalID: string, directory: string, signal?: AbortSignal) =>
    request<FileListing>(`/terminals/${encoded(terminalID)}/files?path=${encoded(directory)}`, {
      signal,
    }),
  mutateFile: (terminalID: string, mutation: Mutation) =>
    request<void>(`/terminals/${encoded(terminalID)}/files`, json('POST', mutation)),
  usage: (terminalID: string, path: string) =>
    request<TreeUsage>(`/terminals/${encoded(terminalID)}/usage?path=${encoded(path)}`),
  document: (terminalID: string, path: string, signal?: AbortSignal) =>
    request<RemoteDocument>(`/terminals/${encoded(terminalID)}/document?path=${encoded(path)}`, {
      signal,
    }),
  saveDocument: (terminalID: string, document: RemoteDocument) =>
    request<RemoteDocument>(`/terminals/${encoded(terminalID)}/document`, json('PUT', document)),
  localDocument: (documentID: string, signal?: AbortSignal) =>
    request<LocalDocument>(`/local-documents/${encoded(documentID)}`, { signal }),
  saveLocalDocument: (documentID: string, document: RemoteDocument) =>
    request<LocalDocument>(`/local-documents/${encoded(documentID)}`, json('PUT', document)),
  downloadURL: (terminalID: string, path: string) =>
    `/api/v1/terminals/${encoded(terminalID)}/download?path=${encoded(path)}`,
  archiveURL: (terminalID: string, path: string) =>
    `/api/v1/terminals/${encoded(terminalID)}/archive?path=${encoded(path)}`,
  vault: () => request<VaultStatus>('/vault'),
  setupVault: (mode: 'keyring' | 'password', password?: string) =>
    request<VaultStatus>('/vault/setup', json('POST', { mode, password })),
  unlockVault: (password?: string) =>
    request<VaultStatus>('/vault/unlock', json('POST', { password })),
  lockVault: () => request<VaultStatus>('/vault/lock', { method: 'POST' }),
  resetVault: () =>
    request<VaultStatus>('/vault/reset', json('POST', { confirm: 'DELETE ALL SAVED CREDENTIALS' })),
  credentials: () => request<CredentialRecord[]>('/credentials'),
  createCredential: (label: string, credentials: Credentials) =>
    request<CredentialRecord>('/credentials', json('POST', { label, credentials })),
  deleteCredential: (credentialID: string) =>
    request<void>(`/credentials/${encoded(credentialID)}`, { method: 'DELETE' }),
  tunnels: () => request<Tunnel[]>('/tunnels'),
  saveTunnel: (input: TunnelInput, tunnelID?: string) =>
    request<Tunnel>(
      `/tunnels${tunnelID ? `/${encoded(tunnelID)}` : ''}`,
      json(tunnelID ? 'PUT' : 'POST', input),
    ),
  deleteTunnel: (tunnelID: string) =>
    request<void>(`/tunnels/${encoded(tunnelID)}`, { method: 'DELETE' }),
  stopTunnel: (tunnelID: string) =>
    request<void>(`/tunnels/${encoded(tunnelID)}/stop`, { method: 'POST' }),
  snippets: () => request<Snippet[]>('/snippets'),
  saveSnippet: (input: SnippetInput, snippetID?: string) =>
    request<Snippet>(
      `/snippets${snippetID ? `/${encoded(snippetID)}` : ''}`,
      json(snippetID ? 'PUT' : 'POST', input),
    ),
  deleteSnippet: (snippetID: string) =>
    request<void>(`/snippets/${encoded(snippetID)}`, { method: 'DELETE' }),
  workspace: () => request<Workspace>('/workspace'),
  saveWorkspace: (workspace: Workspace) => request<Workspace>('/workspace', json('PUT', workspace)),
  sshConfigHosts: () => request<SSHConfigHosts>('/ssh-config/hosts'),
  previewImport: (source: 'local' | 'text', content?: string) =>
    request<ImportPreview>('/imports/ssh-config/preview', json('POST', { source, content })),
  applyImport: (source: 'local' | 'text', aliases: string[], content?: string, group?: string) =>
    request<ImportResult>(
      '/imports/ssh-config/apply',
      json('POST', { source, content, aliases, group }),
    ),
}

/**
 * Streams one file with XHR so progress is observable without buffering the
 * file in JavaScript. `relative` may contain "/" for folder uploads.
 */
export function uploadFile(
  terminalID: string,
  directory: string,
  file: File,
  signal: AbortSignal,
  progress: (percent: number) => void,
  relative = file.name,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const upload = new XMLHttpRequest()
    const abort = () => upload.abort()
    const cleanup = () => signal.removeEventListener('abort', abort)
    upload.open(
      'POST',
      `/api/v1/terminals/${encoded(terminalID)}/upload?path=${encoded(directory)}&name=${encoded(relative)}`,
    )
    upload.setRequestHeader('Content-Type', 'application/octet-stream')
    upload.setRequestHeader('X-CSRF-Token', csrfToken)
    upload.timeout = 30 * 60 * 1000
    upload.upload.onprogress = (event) => {
      if (event.lengthComputable) progress(Math.round((event.loaded / event.total) * 100))
    }
    upload.onload = () => {
      cleanup()
      if (upload.status === 201) {
        resolve()
        return
      }
      let payload: unknown
      try {
        payload = JSON.parse(upload.responseText)
      } catch {
        payload = null
      }
      const problem = apiError(payload)
      if (problem.code === 'unauthenticated')
        window.dispatchEvent(new Event('workbench:unauthenticated'))
      reject(problem)
    }
    upload.onerror = upload.ontimeout = () => {
      cleanup()
      reject(new ApiError('upload_failed', '上传连接中断，远端可能留有不完整文件，请检查后重试。'))
    }
    upload.onabort = () => {
      cleanup()
      reject(
        new ApiError(
          'upload_cancelled',
          '已取消上传；若网络已经断开，请检查远端是否有不完整文件。',
        ),
      )
    }
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) {
      cleanup()
      reject(new ApiError('upload_cancelled', '已取消上传。'))
      return
    }
    upload.send(file)
  })
}
