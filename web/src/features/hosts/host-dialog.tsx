import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LoaderCircle, LockKeyhole, MessageSquareLock, Shield } from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { AuthState, HostInput, HostProfile } from '../../shared/api/types'
import { Modal } from '../../shared/ui/modal'

const emptyHost: HostInput = {
  name: '',
  hostname: '',
  port: 22,
  username: '',
  authMode: 'auto',
  group: '',
  color: 'teal',
  favorite: false,
  production: false,
}

export function HostDialog({
  auth,
  host,
  hosts,
  onClose,
}: {
  auth: AuthState
  host?: HostProfile
  hosts: HostProfile[]
  onClose: () => void
}) {
  const isEditingSaved = !!host && !host.id.startsWith('sshcfg:') && !host.id.startsWith('quick:')
  const [value, setValue] = useState<HostInput>(
    host ? { ...host, group: host.id.startsWith('sshcfg:') ? '' : host.group } : emptyHost,
  )
  const vault = useQuery({ queryKey: ['vault'], queryFn: api.vault })
  const saved = useQuery({
    queryKey: ['credentials'],
    queryFn: api.credentials,
    enabled: !!vault.data?.unlocked,
  })
  const jumpCandidates = hosts.filter(
    (candidate) =>
      !candidate.id.startsWith('sshcfg:') &&
      candidate.id !== (isEditingSaved ? host?.id : undefined) &&
      !candidate.jumpHostId,
  )
  const usedAsJump = isEditingSaved && hosts.some((candidate) => candidate.jumpHostId === host?.id)
  const localFiles = !!auth.capabilities?.localFiles
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const queries = useQueryClient()
  function change<Field extends keyof HostInput>(field: Field, next: HostInput[Field]) {
    setValue((previous) => ({ ...previous, [field]: next }))
  }
  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.saveHost(value, isEditingSaved ? host?.id : undefined)
      await queries.invalidateQueries({ queryKey: ['hosts'] })
      onClose()
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      title={isEditingSaved ? '编辑主机' : '新建 SSH 主机'}
      description="支持自动匹配 SSH Agent 与本机私钥，密码仅在连接时询问或按需保存。"
      onClose={() => {
        if (!busy) onClose()
      }}
    >
      <form onSubmit={(event) => void save(event)}>
        <div className="modal-body form-stack">
          <label>
            显示名称
            <input
              autoFocus
              required
              maxLength={80}
              placeholder="例如：开发服务器"
              value={value.name}
              onChange={(event) => change('name', event.target.value)}
            />
          </label>
          <div className="form-row address-row">
            <label>
              主机地址
              <input
                required
                placeholder="IP 地址或域名"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                value={value.hostname}
                onChange={(event) => change('hostname', event.target.value)}
              />
            </label>
            <label>
              端口
              <input
                required
                type="number"
                min={1}
                max={65535}
                value={value.port}
                onChange={(event) => change('port', Number(event.target.value))}
              />
            </label>
          </div>
          <div className="form-row">
            <label>
              用户名
              <input
                required
                placeholder="例如：ubuntu"
                autoCapitalize="none"
                spellCheck={false}
                value={value.username}
                onChange={(event) => change('username', event.target.value)}
              />
            </label>
            <label>
              分组
              <input
                maxLength={80}
                placeholder="未分组"
                value={value.group}
                onChange={(event) => change('group', event.target.value)}
              />
            </label>
          </div>
          <fieldset>
            <legend>认证方式</legend>
            <div className="choice-row">
              {(
                [
                  { mode: 'auto', label: '自动', icon: Shield },
                  { mode: 'password', label: '密码', icon: LockKeyhole },
                  { mode: 'key', label: '私钥', icon: KeyRound },
                  { mode: 'agent', label: 'Agent', icon: Shield },
                  { mode: 'keyboard-interactive', label: 'MFA', icon: MessageSquareLock },
                ] as const
              ).map(({ mode, label, icon: Icon }) => (
                <button
                  key={mode}
                  type="button"
                  className={`choice ${value.authMode === mode ? 'selected' : ''}`}
                  aria-pressed={value.authMode === mode}
                  onClick={() => change('authMode', mode)}
                >
                  <Icon size={15} />
                  {label}
                </button>
              ))}
            </div>
          </fieldset>
          {value.authMode === 'agent' && !auth.capabilities?.agent && (
            <div className="inline-error">
              远程模式默认禁止服务端 SSH agent，需以 --allow-agent 启动。
            </div>
          )}
          {(value.authMode === 'key' || value.authMode === 'auto') && localFiles && (
            <label>
              本机私钥路径（可选）
              <input
                placeholder="~/.ssh/id_ed25519；留空则自动探测默认密钥与 Agent"
                autoCapitalize="none"
                spellCheck={false}
                value={value.identityFile ?? ''}
                onChange={(event) => change('identityFile', event.target.value)}
              />
            </label>
          )}
          {localFiles && (
            <label>
              ProxyCommand（可选）
              <input
                placeholder="例如：cloudflared access ssh --hostname %h"
                autoCapitalize="none"
                spellCheck={false}
                value={value.proxyCommand ?? ''}
                onChange={(event) => change('proxyCommand', event.target.value)}
              />
            </label>
          )}
          <div className="form-row">
            <label>
              跳板机（ProxyJump，单级）
              <select
                value={value.jumpHostId ?? ''}
                disabled={usedAsJump}
                onChange={(event) => change('jumpHostId', event.target.value)}
              >
                <option value="">直接连接</option>
                {jumpCandidates.map((candidate) => (
                  <option key={candidate.id} value={candidate.id}>
                    {candidate.name} · {candidate.username}@{candidate.hostname}:{candidate.port}
                  </option>
                ))}
              </select>
              {usedAsJump && (
                <span className="field-note">此主机已被用作跳板，不能再设置跳板。</span>
              )}
            </label>
            {(value.authMode === 'auto' ||
              value.authMode === 'password' ||
              value.authMode === 'key') && (
              <label>
                保存的凭据
                <select
                  value={value.credentialId ?? ''}
                  disabled={!vault.data?.unlocked}
                  onChange={(event) => change('credentialId', event.target.value)}
                >
                  <option value="">连接时询问</option>
                  {value.credentialId &&
                    !saved.data?.some((record) => record.id === value.credentialId) && (
                      <option value={value.credentialId}>已关联的凭据</option>
                    )}
                  {saved.data?.map((record) => (
                    <option key={record.id} value={record.id}>
                      {record.label}（{record.kind === 'key' ? '私钥' : '密码'}）
                    </option>
                  ))}
                </select>
                {!vault.data?.unlocked && (
                  <span className="field-note">在设置 → 凭据库中启用或解锁后可选择。</span>
                )}
              </label>
            )}
          </div>
          <div className="form-row">
            <fieldset>
              <legend>主机标记</legend>
              <div className="color-options">
                {(['teal', 'blue', 'amber', 'rose'] as const).map((color) => (
                  <button
                    key={color}
                    type="button"
                    className={`color-option host-${color} ${value.color === color ? 'selected' : ''}`}
                    aria-label={`${color} 标记`}
                    aria-pressed={value.color === color}
                    onClick={() => change('color', color)}
                  />
                ))}
              </div>
            </fieldset>
            <div className="host-flags">
              <label className="checkbox-label favorite-check">
                <input
                  type="checkbox"
                  checked={value.favorite}
                  onChange={(event) => change('favorite', event.target.checked)}
                />
                加入收藏
              </label>
              <label className="checkbox-label favorite-check">
                <input
                  type="checkbox"
                  checked={!!value.production}
                  onChange={(event) => change('production', event.target.checked)}
                />
                生产主机（醒目标记）
              </label>
            </div>
          </div>
          {error && (
            <div className="inline-error" role="alert">
              {error}
            </div>
          )}
        </div>
        <footer className="modal-footer">
          <button type="button" className="button secondary" disabled={busy} onClick={onClose}>
            取消
          </button>
          <button type="submit" className="button primary" disabled={busy}>
            {busy && <LoaderCircle size={16} className="spin" />}
            {isEditingSaved ? '保存修改' : '创建主机'}
          </button>
        </footer>
      </form>
    </Modal>
  )
}
