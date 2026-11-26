import { useMemo, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, MapPin } from 'lucide-react'
import { api, messageOf } from '../../shared/api/client'
import type { HostProfile, Tunnel } from '../../shared/api/types'
import { Modal } from '../../shared/ui/modal'
import {
  emptyTunnelForm,
  endpoint,
  listenHosts,
  listenLocationNote,
  listenPortHint,
  normalizeTargetHost,
  parsePort,
  tunnelFormOf,
  validateTunnelForm,
  type TunnelForm,
} from './tunnel-form'
import './tunnels.css'

/** Stops implicit form submission when Enter only commits an IME composition. */
function holdComposingEnter(event: KeyboardEvent<HTMLFormElement>) {
  if (event.key === 'Enter' && (event.nativeEvent.isComposing || event.keyCode === 229))
    event.preventDefault()
}

function hostLabel(host: HostProfile, hosts: readonly HostProfile[]) {
  const jump = host.jumpHostId ? hosts.find((item) => item.id === host.jumpHostId) : undefined
  return `${host.name} · ${host.username}@${host.hostname}:${host.port}${
    jump ? ` · 经跳板 ${jump.name}` : ''
  }${host.production ? ' · 生产' : ''}`
}

export function TunnelDialog({
  remote,
  hosts,
  tunnel,
  onClose,
}: {
  remote: boolean
  hosts: HostProfile[]
  tunnel?: Tunnel
  onClose: () => void
}) {
  const [form, setForm] = useState<TunnelForm>(() =>
    tunnel ? tunnelFormOf(tunnel) : emptyTunnelForm(hosts),
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const queries = useQueryClient()
  const hostIDs = useMemo(() => new Set(hosts.map((host) => host.id)), [hosts])
  const selectedHost = hosts.find((host) => host.id === form.hostId)
  const listenPort = parsePort(form.listenPort)
  const targetPort = parsePort(form.targetPort)
  const portHint = listenPortHint(form.listenPort)
  const side = remote ? '服务器' : '本地'

  function change<Field extends keyof TunnelForm>(field: Field, next: TunnelForm[Field]) {
    setForm((previous) => ({ ...previous, [field]: next }))
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    const checked = validateTunnelForm(form, hostIDs)
    if (checked.error !== undefined) {
      setError(checked.error)
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.saveTunnel(checked.input, tunnel?.id)
      await queries.invalidateQueries({ queryKey: ['tunnels'] })
      onClose()
    } catch (problem) {
      setError(messageOf(problem))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      title={tunnel ? '编辑隧道' : '新建隧道'}
      description="本地转发（-L）：连接监听端口的程序，会经由所选 SSH 主机连到目标地址。"
      onClose={() => {
        if (!busy) onClose()
      }}
    >
      <form
        className="tunnel-form"
        noValidate
        onKeyDown={holdComposingEnter}
        onSubmit={(event) => void save(event)}
      >
        <div className="modal-body form-stack">
          <label>
            名称
            <input
              autoFocus
              required
              maxLength={80}
              placeholder="例如：生产数据库"
              value={form.name}
              onChange={(event) => change('name', event.target.value)}
            />
          </label>
          <label>
            SSH 主机
            <select
              required
              value={form.hostId}
              onChange={(event) => change('hostId', event.target.value)}
            >
              <option value="" disabled>
                {hosts.length === 0 ? '暂无主机，请先添加 SSH 主机' : '请选择经由的 SSH 主机'}
              </option>
              {hosts.map((host) => (
                <option key={host.id} value={host.id}>
                  {hostLabel(host, hosts)}
                </option>
              ))}
            </select>
          </label>
          <div className="form-row">
            <label>
              监听地址
              <select
                value={form.listenHost}
                onChange={(event) =>
                  change('listenHost', event.target.value === '::1' ? '::1' : listenHosts[0])
                }
              >
                <option value="127.0.0.1">127.0.0.1（IPv4 回环）</option>
                <option value="::1">::1（IPv6 回环）</option>
              </select>
            </label>
            <label>
              {remote ? '监听端口（服务器）' : '本地端口'}
              <input
                required
                type="number"
                inputMode="numeric"
                min={1}
                max={65535}
                step={1}
                placeholder="例如：15432"
                value={form.listenPort}
                onChange={(event) => change('listenPort', event.target.value)}
              />
            </label>
          </div>
          <div className="info-box">
            <MapPin size={16} />
            <span>{listenLocationNote(remote)}</span>
          </div>
          <div className="form-row tunnel-target-row">
            <label>
              目标主机
              <input
                required
                maxLength={253}
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                placeholder="127.0.0.1"
                value={form.targetHost}
                onChange={(event) => change('targetHost', event.target.value)}
              />
            </label>
            <label>
              目标端口
              <input
                required
                type="number"
                inputMode="numeric"
                min={1}
                max={65535}
                step={1}
                placeholder="例如：5432"
                value={form.targetPort}
                onChange={(event) => change('targetPort', event.target.value)}
              />
            </label>
          </div>
          <p className="form-hint">
            目标地址在 SSH 主机一侧解析：127.0.0.1 指 SSH 主机自身，内网域名按该主机的 DNS 解析。
            {portHint && ` ${portHint}`}
          </p>
          {selectedHost && listenPort !== undefined && targetPort !== undefined && (
            <p className="tunnel-preview">
              {side} {endpoint(form.listenHost, listenPort)} → 经 {selectedHost.name} → 目标{' '}
              {endpoint(normalizeTargetHost(form.targetHost) || '?', targetPort)}
            </p>
          )}
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
            {tunnel ? '保存修改' : '创建隧道'}
          </button>
        </footer>
      </form>
    </Modal>
  )
}
