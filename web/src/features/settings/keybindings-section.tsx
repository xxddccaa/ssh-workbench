import { useEffect, useState } from 'react'
import type { Preferences } from '../../shared/api/types'
import { actions, bindings, describe, record, type ActionID } from '../../shared/lib/keybindings'
import {
  applyRecorded,
  findConflict,
  isOverridden,
  missingModifierHint,
  withBinding,
  withoutOverride,
  type Overrides,
} from './keybinding-overrides'
import { SettingsHeading } from './settings-heading'

const modifierKeys = new Set([
  'Shift',
  'Control',
  'Alt',
  'AltGraph',
  'Meta',
  'OS',
  'Fn',
  'CapsLock',
])

/**
 * Space activates a focused button on keyup. After a recording finishes with
 * Space, swallow that keyup so it does not immediately restart recording.
 */
function swallowNextSpaceKeyUp() {
  const swallow = (event: KeyboardEvent) => {
    if (event.code !== 'Space') return
    event.preventDefault()
    event.stopPropagation()
    stop()
  }
  const timer = window.setTimeout(() => stop(), 1500)
  function stop() {
    window.clearTimeout(timer)
    window.removeEventListener('keyup', swallow, true)
  }
  window.addEventListener('keyup', swallow, true)
}

export function KeybindingsSection({
  preferences,
  onPreferences,
}: {
  preferences: Preferences
  onPreferences: (next: Preferences) => void
}) {
  const overrides: Overrides = preferences.keybindings
  const current = bindings(overrides)
  const [recording, setRecording] = useState<ActionID | null>(null)
  const [problem, setProblem] = useState<{ id: ActionID; message: string } | null>(null)

  function save(next: Overrides) {
    onPreferences({ ...preferences, keybindings: next })
  }

  function stopRecording() {
    setRecording(null)
    setProblem(null)
  }

  useEffect(() => {
    if (!recording) return
    const listener = (event: KeyboardEvent) => {
      if (event.isComposing || event.keyCode === 229 || event.key === 'Process') return
      const commandless = !event.ctrlKey && !event.metaKey && !event.altKey
      if (event.key === 'Tab' && commandless) {
        // Let keyboard users leave the row; the button's blur ends recording.
        return
      }
      event.preventDefault()
      event.stopPropagation()
      if (event.key === 'Escape' && commandless) {
        setRecording(null)
        setProblem(null)
        return
      }
      if (modifierKeys.has(event.key)) return
      const keys = record(event)
      if (!keys) {
        const modified = event.ctrlKey || event.metaKey || event.altKey || event.shiftKey
        setProblem({
          id: recording,
          message: modified ? '无法识别这个按键，请换一个。' : missingModifierHint(),
        })
        return
      }
      const outcome = applyRecorded(preferences.keybindings, recording, keys)
      if ('error' in outcome) {
        setProblem({ id: recording, message: outcome.error })
        return
      }
      if (event.code === 'Space') swallowNextSpaceKeyUp()
      onPreferences({ ...preferences, keybindings: outcome.overrides })
      setRecording(null)
      setProblem(null)
    }
    window.addEventListener('keydown', listener, true)
    return () => window.removeEventListener('keydown', listener, true)
  }, [recording, preferences, onPreferences])

  function restoreDefault(id: ActionID) {
    const action = actions.find((item) => item.id === id)
    if (!action) return
    const conflict = findConflict(overrides, id, action.defaultKeys)
    if (conflict) {
      setRecording(null)
      setProblem({
        id,
        message: `默认组合 ${describe(action.defaultKeys)} 已用于「${conflict.label}」。请先修改或清除那一项。`,
      })
      return
    }
    stopRecording()
    save(withoutOverride(overrides, id))
  }

  return (
    <>
      <SettingsHeading title="快捷键">
        点击“修改”后按下新的组合键，Esc 取消。设置保存在工作区。
      </SettingsHeading>
      <p className="form-hint">
        浏览器会保留部分快捷键（如
        Ctrl/⌘+W、⌘+T），网页无法拦截，也无法在这里录制；输入法组字期间快捷键不会触发。
      </p>
      <div className="settings-keys-toolbar">
        <button
          type="button"
          className="button secondary small"
          disabled={Object.keys(overrides).length === 0}
          onClick={() => {
            if (!window.confirm('将所有快捷键恢复为默认设置？')) return
            stopRecording()
            save({})
          }}
        >
          全部恢复默认
        </button>
      </div>
      <table className="settings-keys">
        <thead>
          <tr>
            <th scope="col">操作</th>
            <th scope="col">快捷键</th>
            <th scope="col">
              <span className="visually-hidden">修改</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {actions.map((action) => {
            const keys = current[action.id]
            const overridden = isOverridden(overrides, action.id)
            const active = recording === action.id
            const message = problem?.id === action.id ? problem.message : ''
            return (
              <tr key={action.id} className={active ? 'recording' : undefined}>
                <th scope="row">{action.label}</th>
                <td>
                  <div className="settings-key-value">
                    {active ? (
                      <span className="settings-recording" aria-live="polite">
                        请按下新的组合键…
                      </span>
                    ) : keys ? (
                      <kbd>{describe(keys)}</kbd>
                    ) : (
                      <span className="settings-unbound">未设置</span>
                    )}
                    {overridden && !active && (
                      <span className="settings-default">默认 {describe(action.defaultKeys)}</span>
                    )}
                  </div>
                  {message && (
                    <p className="settings-key-error" role="alert">
                      {message}
                    </p>
                  )}
                </td>
                <td>
                  <div className="settings-key-actions">
                    <button
                      type="button"
                      className="button secondary small"
                      aria-pressed={active}
                      aria-label={
                        active ? `取消录制（${action.label}）` : `修改（${action.label}）`
                      }
                      onClick={() => {
                        if (active) stopRecording()
                        else {
                          setProblem(null)
                          setRecording(action.id)
                        }
                      }}
                      onBlur={() => {
                        if (active) setRecording(null)
                      }}
                    >
                      {active ? '取消' : '修改'}
                    </button>
                    <button
                      type="button"
                      className="text-button"
                      aria-label={`清除（${action.label}）`}
                      disabled={!keys}
                      onClick={() => {
                        stopRecording()
                        save(withBinding(overrides, action.id, ''))
                      }}
                    >
                      清除
                    </button>
                    <button
                      type="button"
                      className="text-button"
                      aria-label={`恢复默认（${action.label}）`}
                      disabled={!overridden}
                      onClick={() => restoreDefault(action.id)}
                    >
                      恢复默认
                    </button>
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </>
  )
}
