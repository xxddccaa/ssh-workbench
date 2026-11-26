import { useId, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { Check, CircleAlert, CircleMinus, Eye, PencilLine, Send, TriangleAlert } from 'lucide-react'
import type { Snippet } from '../../shared/api/types'
import {
  byteLength,
  commandByteLimit,
  commandLines,
  confirmationMatches,
  outcomeLabel,
  planBroadcast,
  sendOnce,
  tally,
  trimTrailingNewlines,
  type BroadcastPlan,
  type BroadcastResult,
  type Sendable,
} from './commands'

export interface Prefill {
  command: string
  serial: number
}

type Phase =
  | { step: 'compose' }
  | { step: 'preview'; plan: BroadcastPlan }
  | { step: 'results'; plan: BroadcastPlan; results: BroadcastResult[] }

/** Stops implicit form submission when Enter only commits an IME composition. */
function holdComposingEnter(event: KeyboardEvent<HTMLFormElement>) {
  if (event.target instanceof HTMLTextAreaElement) return
  if (event.key === 'Enter' && (event.nativeEvent.isComposing || event.keyCode === 229))
    event.preventDefault()
}

function ProductionBadge() {
  return <span className="production-badge">生产</span>
}

export function BroadcastComposer({
  targets,
  snippets,
  prefill,
  onClose,
}: {
  targets: readonly Sendable[]
  snippets: readonly Snippet[]
  prefill?: Prefill
  onClose: () => void
}) {
  const [command, setCommand] = useState(prefill?.command ?? '')
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [phase, setPhase] = useState<Phase>({ step: 'compose' })
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [appliedPrefill, setAppliedPrefill] = useState(prefill?.serial)
  // Plans that were already sent: a repeated submit (double click, re-render) is a no-op.
  const sent = useRef(new WeakSet<BroadcastPlan>())
  const counterID = useId()
  const targetsHeadingID = useId()

  // "用于广播" from the snippet tab replaces the draft and returns to editing; it never sends.
  if (prefill && prefill.serial !== appliedPrefill) {
    setAppliedPrefill(prefill.serial)
    setCommand(prefill.command)
    setPhase({ step: 'compose' })
    setConfirmation('')
    setError('')
  }

  const liveTargets = new Map(targets.map((target) => [target.id, target]))
  const chosenCount = targets.filter((target) => selected.has(target.id)).length
  const size = byteLength(trimTrailingNewlines(command))
  const lines = commandLines(command).length

  function toggle(id: string, checked: boolean) {
    setSelected((previous) => {
      const next = new Set(previous)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  function preview() {
    const planned = planBroadcast(command, targets, selected)
    if (planned.error !== undefined) {
      setError(planned.error)
      return
    }
    setError('')
    setConfirmation('')
    setPhase({ step: 'preview', plan: planned.plan })
  }

  function send(event: FormEvent) {
    event.preventDefault()
    if (phase.step !== 'preview') return
    const { plan } = phase
    if (!confirmationMatches(confirmation, plan.targets.length)) return
    const results = sendOnce(plan, sent.current, (id) => liveTargets.get(id))
    if (!results) return
    setPhase({ step: 'results', plan, results })
  }

  function editAgain() {
    setConfirmation('')
    setPhase({ step: 'compose' })
  }

  if (phase.step === 'results') {
    const { sent: delivered, unsent } = tally(phase.results)
    return (
      <>
        <div className="modal-body form-stack">
          <div className={`broadcast-summary ${unsent ? 'partial' : ''}`} role="status">
            {unsent === 0 ? <Check size={17} /> : <CircleAlert size={17} />}
            已发送到 {delivered} 台，未发送 {unsent} 台。
          </div>
          <ul className="broadcast-hosts" aria-label="发送结果">
            {phase.results.map((result) => (
              <li
                key={result.id}
                className={`${result.production ? 'production' : ''} ${result.outcome}`}
              >
                {result.outcome === 'sent' ? (
                  <Check size={15} className="broadcast-ok" />
                ) : result.outcome === 'error' ? (
                  <CircleAlert size={15} className="broadcast-bad" />
                ) : (
                  <CircleMinus size={15} className="broadcast-bad" />
                )}
                <strong>{result.name}</strong>
                <code>{result.address}</code>
                {result.production && <ProductionBadge />}
                <span className="broadcast-outcome">{outcomeLabel[result.outcome]}</span>
              </li>
            ))}
          </ul>
          <p className="form-hint">
            “已发送”只表示整条命令已交给该终端连接，不代表执行成功，请在各终端中查看输出。未发送的主机不会自动重试。
          </p>
        </div>
        <footer className="modal-footer">
          <button type="button" className="button secondary" onClick={editAgain}>
            <PencilLine size={15} />
            再次编辑
          </button>
          <button type="button" className="button primary" onClick={onClose}>
            关闭
          </button>
        </footer>
      </>
    )
  }

  if (phase.step === 'preview') {
    const { plan } = phase
    const count = plan.targets.length
    const confirmed = confirmationMatches(confirmation, count)
    return (
      <form noValidate onKeyDown={holdComposingEnter} onSubmit={send}>
        <div className="modal-body form-stack">
          <section className="broadcast-section">
            <h3>
              将发送的命令
              <span>
                {plan.lineCount > 1
                  ? `${plan.lineCount} 行，逐行按顺序执行，末尾自动回车`
                  : '末尾自动回车'}
              </span>
            </h3>
            <pre className="broadcast-command">{plan.command}</pre>
          </section>
          <section className="broadcast-section">
            <h3>
              目标主机 {count} 台
              {plan.productionCount > 0 && (
                <span className="broadcast-production-count">
                  其中 {plan.productionCount} 台为生产主机
                </span>
              )}
            </h3>
            <ul className="broadcast-hosts" aria-label="目标主机">
              {plan.targets.map((target) => {
                const live = liveTargets.get(target.id)
                return (
                  <li key={target.id} className={target.production ? 'production' : ''}>
                    <strong>{target.name}</strong>
                    <code>{target.address}</code>
                    {target.production && <ProductionBadge />}
                    {!live ? (
                      <span className="subtle-badge">已关闭，将跳过</span>
                    ) : (
                      !live.ready() && <span className="subtle-badge">未就绪，将跳过</span>
                    )}
                  </li>
                )
              })}
            </ul>
          </section>
          <div className="broadcast-warning">
            <TriangleAlert size={17} />
            <ul>
              <li>
                SSH Workbench 无法判断每个终端当前是否停在 shell 提示符。若某台主机正在运行
                vim、less、密码提示或其他交互程序，命令会作为普通按键输入给该程序。
              </li>
              <li>每台主机只发送一次，不会重试；发送时连接未就绪的终端会被跳过并标记。</li>
              <li>命令发出后无法撤回。</li>
            </ul>
          </div>
          <label>
            请输入目标主机数量 {count} 以确认
            <input
              autoFocus
              inputMode="numeric"
              autoComplete="off"
              placeholder={String(count)}
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </label>
        </div>
        <footer className="modal-footer">
          <button type="button" className="button secondary" onClick={editAgain}>
            返回修改
          </button>
          <button
            type="submit"
            className={`button ${plan.productionCount > 0 ? 'danger' : 'primary'}`}
            disabled={!confirmed}
          >
            <Send size={15} />
            发送到 {count} 台主机
          </button>
        </footer>
      </form>
    )
  }

  return (
    <>
      <div className="modal-body form-stack">
        <p className="muted-copy">
          把整条命令一次性发送到勾选的终端（不是逐键同步输入）。发送前会预览命令与目标，并要求输入目标数量确认。
        </p>
        {snippets.length > 0 && (
          <label>
            从命令片段填入
            <select
              className="snippet-select"
              value=""
              onChange={(event) => {
                const picked = snippets.find((snippet) => snippet.id === event.target.value)
                if (!picked) return
                if (
                  command.trim() &&
                  command !== picked.command &&
                  !window.confirm('用所选片段替换当前输入的命令？')
                )
                  return
                setCommand(picked.command)
                setError('')
              }}
            >
              <option value="">选择片段…</option>
              {snippets.map((snippet) => (
                <option key={snippet.id} value={snippet.id}>
                  {snippet.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <div className="snippet-field">
          <label>
            命令
            <textarea
              rows={5}
              spellCheck={false}
              autoCapitalize="none"
              autoCorrect="off"
              placeholder="例如：uptime"
              aria-describedby={counterID}
              value={command}
              onChange={(event) => setCommand(event.target.value)}
            />
          </label>
          <span
            id={counterID}
            className={`snippet-counter ${size > commandByteLimit ? 'over' : ''}`}
          >
            {size} / {commandByteLimit} 字节
            {lines > 1 ? ` · ${lines} 行，每台主机上逐行按顺序执行` : ''}
          </span>
        </div>
        <div role="group" aria-labelledby={targetsHeadingID}>
          <div className="broadcast-legend">
            <span id={targetsHeadingID}>
              目标终端（已选 {chosenCount} / {targets.length}）
            </span>
            {targets.length > 0 && (
              <span>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => setSelected(new Set(targets.map((target) => target.id)))}
                >
                  全选
                </button>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => setSelected(new Set())}
                >
                  清空
                </button>
              </span>
            )}
          </div>
          {targets.length === 0 ? (
            <p className="snippet-empty">没有打开的终端。先连接主机，再回来选择广播目标。</p>
          ) : (
            <ul className="broadcast-targets">
              {targets.map((target) => (
                <li key={target.id} className={target.production ? 'production' : ''}>
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={selected.has(target.id)}
                      onChange={(event) => toggle(target.id, event.target.checked)}
                    />
                    <strong>{target.name}</strong>
                    <code>{target.address}</code>
                    {target.production && <ProductionBadge />}
                    {!target.ready() && <span className="subtle-badge">未就绪</span>}
                  </label>
                </li>
              ))}
            </ul>
          )}
        </div>
        {error && (
          <div className="inline-error" role="alert">
            {error}
          </div>
        )}
      </div>
      <footer className="modal-footer">
        <button type="button" className="button secondary" onClick={onClose}>
          取消
        </button>
        <button
          type="button"
          className="button primary"
          disabled={!command.trim() || chosenCount === 0}
          title={
            !command.trim()
              ? '请先输入命令'
              : chosenCount === 0
                ? '请至少勾选一台目标主机'
                : '预览命令与目标，下一步确认后才会发送'
          }
          onClick={preview}
        >
          <Eye size={15} />
          预览
        </button>
      </footer>
    </>
  )
}
