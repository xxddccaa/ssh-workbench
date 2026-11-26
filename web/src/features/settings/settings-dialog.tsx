import { useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  Bot,
  FileKey,
  Info,
  Keyboard,
  KeyRound,
  LockKeyhole,
  Minus,
  Moon,
  Palette,
  Plus,
  ShieldCheck,
  SquareTerminal,
  Sun,
  type LucideIcon,
} from 'lucide-react'
import type { AuthState, Preferences } from '../../shared/api/types'
import { Modal } from '../../shared/ui/modal'
import { KeybindingsSection } from './keybindings-section'
import { clampInteger, fontSizeRange, parseInRange, scrollbackRange } from './preferences'
import { SettingsHeading } from './settings-heading'
import { ShellSection } from './shell-section'
import { VaultSection } from './vault-section'
import './settings.css'

export type SettingsSection = 'appearance' | 'keys' | 'vault' | 'shell' | 'security'

const sections: { id: SettingsSection; label: string; icon: LucideIcon }[] = [
  { id: 'appearance', label: '外观', icon: Palette },
  { id: 'keys', label: '快捷键', icon: Keyboard },
  { id: 'vault', label: '凭据库', icon: KeyRound },
  { id: 'shell', label: 'Shell 集成', icon: SquareTerminal },
  { id: 'security', label: '安全与会话', icon: ShieldCheck },
]

export function SettingsDialog({
  auth,
  theme,
  toggleTheme,
  preferences,
  onPreferences,
  initialSection = 'appearance',
  canCloseEditor,
  onLogout,
  onClose,
}: {
  auth: AuthState
  theme: 'dark' | 'light'
  toggleTheme: () => void
  preferences: Preferences
  onPreferences: (next: Preferences) => void
  initialSection?: SettingsSection
  canCloseEditor: () => boolean
  onLogout: () => void
  onClose: () => void
}) {
  const [section, setSection] = useState<SettingsSection>(initialSection)
  const tabs = useRef(new Map<SettingsSection, HTMLButtonElement>())

  function navigate(event: KeyboardEvent<HTMLDivElement>) {
    if (event.nativeEvent.isComposing) return
    const index = sections.findIndex((item) => item.id === section)
    const target =
      event.key === 'ArrowDown'
        ? sections[(index + 1) % sections.length]
        : event.key === 'ArrowUp'
          ? sections[(index - 1 + sections.length) % sections.length]
          : event.key === 'Home'
            ? sections[0]
            : event.key === 'End'
              ? sections[sections.length - 1]
              : undefined
    if (!target) return
    event.preventDefault()
    setSection(target.id)
    tabs.current.get(target.id)?.focus()
  }

  return (
    <Modal title="设置" description={`SSH Workbench ${auth.version}`} onClose={onClose} wide>
      <div className="settings-layout">
        <div
          className="settings-nav"
          role="tablist"
          aria-label="设置分类"
          aria-orientation="vertical"
          onKeyDown={navigate}
        >
          {sections.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              ref={(element) => {
                if (element) tabs.current.set(id, element)
                else tabs.current.delete(id)
              }}
              type="button"
              role="tab"
              id={`settings-tab-${id}`}
              aria-selected={section === id}
              aria-controls="settings-panel"
              tabIndex={section === id ? 0 : -1}
              className={section === id ? 'selected' : ''}
              onClick={() => setSection(id)}
            >
              <Icon size={16} />
              {label}
            </button>
          ))}
        </div>
        <div
          className="settings-pane form-stack"
          role="tabpanel"
          id="settings-panel"
          aria-labelledby={`settings-tab-${section}`}
          tabIndex={0}
        >
          {section === 'appearance' && (
            <AppearanceSection
              theme={theme}
              toggleTheme={toggleTheme}
              preferences={preferences}
              onPreferences={onPreferences}
            />
          )}
          {section === 'keys' && (
            <KeybindingsSection preferences={preferences} onPreferences={onPreferences} />
          )}
          {section === 'vault' && <VaultSection auth={auth} />}
          {section === 'shell' && <ShellSection />}
          {section === 'security' && (
            <SecuritySection auth={auth} canCloseEditor={canCloseEditor} onLogout={onLogout} />
          )}
        </div>
      </div>
      <footer className="modal-footer">
        <button type="button" className="button secondary" onClick={onClose}>
          完成
        </button>
      </footer>
    </Modal>
  )
}

const cursorStyles: { id: Preferences['cursorStyle']; label: string }[] = [
  { id: 'bar', label: '竖线' },
  { id: 'block', label: '方块' },
  { id: 'underline', label: '下划线' },
]

function AppearanceSection({
  theme,
  toggleTheme,
  preferences,
  onPreferences,
}: {
  theme: 'dark' | 'light'
  toggleTheme: () => void
  preferences: Preferences
  onPreferences: (next: Preferences) => void
}) {
  const [scrollbackDraft, setScrollbackDraft] = useState<string | null>(null)
  const fontSize = clampInteger(preferences.fontSize, fontSizeRange)

  function update(change: Partial<Preferences>) {
    onPreferences({ ...preferences, ...change })
  }

  function commitScrollback() {
    if (scrollbackDraft === null) return
    const typed = Number(scrollbackDraft.trim())
    if (scrollbackDraft.trim() && Number.isFinite(typed))
      update({ scrollback: clampInteger(typed, scrollbackRange) })
    setScrollbackDraft(null)
  }

  return (
    <>
      <SettingsHeading title="外观">
        界面主题保存在当前浏览器；终端设置保存在工作区。
      </SettingsHeading>
      <div className="settings-field">
        <div>
          <strong>界面主题</strong>
          <span>当前为{theme === 'dark' ? '深色' : '浅色'}主题。</span>
        </div>
        <button type="button" className="button secondary small" onClick={toggleTheme}>
          {theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />}
          {theme === 'dark' ? '切换浅色' : '切换深色'}
        </button>
      </div>
      <div className="settings-field">
        <div>
          <strong id="settings-font-size">终端字号</strong>
          <span>
            {fontSizeRange.min}–{fontSizeRange.max} px
          </span>
        </div>
        <div className="settings-stepper" role="group" aria-labelledby="settings-font-size">
          <button
            type="button"
            className="icon-button bordered"
            aria-label="减小终端字号"
            disabled={fontSize <= fontSizeRange.min}
            onClick={() => update({ fontSize: clampInteger(fontSize - 1, fontSizeRange) })}
          >
            <Minus size={15} />
          </button>
          <output aria-live="polite">{fontSize} px</output>
          <button
            type="button"
            className="icon-button bordered"
            aria-label="增大终端字号"
            disabled={fontSize >= fontSizeRange.max}
            onClick={() => update({ fontSize: clampInteger(fontSize + 1, fontSizeRange) })}
          >
            <Plus size={15} />
          </button>
        </div>
      </div>
      <div className="settings-field">
        <div>
          <strong id="settings-scrollback">回滚行数</strong>
          <span id="settings-scrollback-hint">
            {scrollbackRange.min}–{scrollbackRange.max} 行。行数越多，每个终端占用的浏览器内存越多。
          </span>
        </div>
        <input
          aria-labelledby="settings-scrollback"
          aria-describedby="settings-scrollback-hint"
          className="settings-number"
          type="number"
          inputMode="numeric"
          min={scrollbackRange.min}
          max={scrollbackRange.max}
          step={scrollbackRange.step}
          value={scrollbackDraft ?? String(preferences.scrollback)}
          onChange={(event) => {
            setScrollbackDraft(event.target.value)
            const value = parseInRange(event.target.value, scrollbackRange)
            if (value !== null) update({ scrollback: value })
          }}
          onBlur={commitScrollback}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.nativeEvent.isComposing) commitScrollback()
          }}
        />
      </div>
      <fieldset className="settings-fieldset">
        <legend>光标样式</legend>
        <div className="choice-row">
          {cursorStyles.map((style) => (
            <button
              key={style.id}
              type="button"
              className={`choice ${preferences.cursorStyle === style.id ? 'selected' : ''}`}
              aria-pressed={preferences.cursorStyle === style.id}
              onClick={() => update({ cursorStyle: style.id })}
            >
              <span className={`settings-cursor-sample ${style.id}`} aria-hidden="true" />
              {style.label}
            </button>
          ))}
        </div>
      </fieldset>
    </>
  )
}

function Capability({
  icon: Icon,
  title,
  enabled,
  children,
}: {
  icon: LucideIcon
  title: string
  enabled: boolean | undefined
  children: ReactNode
}) {
  return (
    <div className="setting-detail">
      <Icon size={20} />
      <div>
        <strong>
          {title}
          <span className={`settings-state ${enabled ? 'on' : 'off'}`}>
            {enabled === undefined ? '服务未报告' : enabled ? '已允许' : '未开放'}
          </span>
        </strong>
        <p>{children}</p>
      </div>
    </div>
  )
}

function SecuritySection({
  auth,
  canCloseEditor,
  onLogout,
}: {
  auth: AuthState
  canCloseEditor: () => boolean
  onLogout: () => void
}) {
  const authRequired = auth.authRequired !== false
  const relock = '锁定后可在终端运行 sshw open 获取新的授权链接，无需重启服务。'
  const capabilities = auth.capabilities
  return (
    <>
      <SettingsHeading title="安全与会话" />
      <div className="setting-detail">
        <ShieldCheck size={20} />
        <div>
          <strong>
            {auth.remote ? '远程 HTTPS 模式' : authRequired ? '本机授权模式' : '开箱直连模式'}
          </strong>
          <p>
            {auth.remote
              ? '只能通过配置的 HTTPS 来源访问。密码登录、CSRF 与主机密钥校验均开启。'
              : authRequired
                ? '使用一次性授权链接或密码建立会话。'
                : '当前为直接打开模式：访问服务地址即可进入工作区。如需密码/HTTPS 保护，可在启动时加上 --auth 或 --remote。'}
          </p>
        </div>
      </div>
      <div className="setting-detail">
        <LockKeyhole size={20} />
        <div>
          <strong>SSH 凭据默认只保留在当前会话内存</strong>
          <p>
            连接时输入的密码和私钥只用于本次连接。只有启用凭据库并主动选择保存时，凭据才会加密写入本机数据库；保存的内容不会再显示到浏览器。
          </p>
        </div>
      </div>
      <Capability icon={Bot} title="系统 SSH agent" enabled={capabilities?.agent}>
        {capabilities?.agent
          ? '可使用服务所在机器的 SSH agent 认证。不会启用 agent 转发，私钥不会发往远端。'
          : auth.remote
            ? '远程模式默认关闭；服务所有者须以 --allow-agent 启动才会开放。'
            : '当前服务没有开放 SSH agent。'}
      </Capability>
      <Capability icon={FileKey} title="本机 SSH Config 与私钥" enabled={capabilities?.localFiles}>
        {capabilities?.localFiles
          ? '已开启本机 ~/.ssh/config 自动识别、ProxyCommand 与本地私钥读取，支持软链接。'
          : '服务不读取所在机器的 ~/.ssh 文件。请在浏览器中选择私钥文件，或粘贴配置内容导入。'}
      </Capability>
      {authRequired && (
        <div className="info-box">
          <Info size={19} />
          <span>
            {auth.remote
              ? '退出登录会结束所有终端会话，并取消正在进行的传输。'
              : `锁定工作区会结束所有终端会话，并取消正在进行的传输。${relock}`}
          </span>
        </div>
      )}
      <div className="settings-field">
        <div>
          <strong>版本</strong>
          <span>SSH Workbench {auth.version}</span>
        </div>
        {authRequired && (
          <button
            type="button"
            className="button danger small"
            onClick={() => {
              if (
                !canCloseEditor() ||
                !window.confirm(
                  auth.remote
                    ? '退出登录并结束所有当前会话和传输？'
                    : `锁定将结束当前会话和传输。${relock}继续？`,
                )
              )
                return
              onLogout()
            }}
          >
            <LockKeyhole size={15} />
            {auth.remote ? '退出登录' : '锁定工作区'}
          </button>
        )}
      </div>
    </>
  )
}
