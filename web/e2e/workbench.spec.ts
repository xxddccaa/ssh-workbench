import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { connect } from 'node:net'
import { join } from 'node:path'

const sshPort = () => process.env.SSHWB_SSH_PORT!
const ssh2Port = () => process.env.SSHWB_SSH2_PORT!
const echoPort = () => process.env.SSHWB_ECHO_PORT!

async function echoThrough(port: number, message: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const socket = connect({ host: '127.0.0.1', port }, () => socket.write(message))
    let received = ''
    socket.setTimeout(5000, () => reject(new Error('tunnel timeout')))
    socket.on('data', (data) => {
      received += data.toString('utf8')
      if (received.length >= message.length) {
        socket.end()
        resolve(received)
      }
    })
    socket.on('error', reject)
  })
}

async function createHost(
  page: Page,
  fields: { name: string; port: string; username: string; group?: string },
  configure?: (dialog: ReturnType<Page['getByRole']>) => Promise<void>,
) {
  await page.getByRole('button', { name: '新建主机', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: '新建 SSH 主机' })
  await dialog.getByLabel('显示名称').fill(fields.name)
  await dialog.getByLabel('主机地址').fill('127.0.0.1')
  await dialog.getByLabel('端口', { exact: true }).fill(fields.port)
  await dialog.getByLabel('用户名', { exact: true }).fill(fields.username)
  if (fields.group) await dialog.getByLabel('分组', { exact: true }).fill(fields.group)
  if (configure) await configure(dialog)
  await dialog.getByRole('button', { name: '创建主机', exact: true }).click()
  await expect(dialog).not.toBeVisible()
}

test('direct-open SSH workbench: ssh config 1-click, terminals, SFTP drawer, MFA, split, broadcast, tunnels', async ({
  page,
  context,
}, testInfo) => {
  const browserErrors: string[] = []
  page.on('pageerror', (error) => browserErrors.push(error.message))
  const baseURL = new URL(process.env.SSHWB_BOOTSTRAP_URL!).origin
  const screenshots = process.env.SSHWB_SCREENSHOTS!
  let terminalID = ''

  await test.step('opens directly without login gate and shows launchpad + ssh config', async () => {
    await page.goto(baseURL)
    await expect(page.getByRole('heading', { name: '快速连接' })).toBeVisible()
    await expect(page.locator('.host-sidebar')).toContainText('fixture-mac-config')
    await page.screenshot({ path: join(screenshots, 'welcome-dark.png') })
    await page.getByRole('button', { name: '切换浅色主题' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await page.screenshot({ path: join(screenshots, 'welcome-light.png') })
    await page.getByRole('button', { name: '切换深色主题' }).click()
  })

  await test.step('1-click connect to ~/.ssh/config host works without any modal', async () => {
    await page.locator('.host-sidebar').getByTitle(`连接 workbench-key@127.0.0.1:${sshPort()}`).click()
    await expect(page.locator('.terminal-pane.active .terminal-toolbar')).toContainText('已连接')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    // SFTP panel is collapsed by default so terminal gets full width.
    await expect(page.locator('.file-panel')).toHaveCount(0)
    await page.getByRole('button', { name: '关闭标签 fixture-mac-config' }).click()
  })

  await test.step('host profile connects with TOFU auto-trust and SFTP drawer toggles on demand', async () => {
    await createHost(
      page,
      {
        name: '中文验收主机',
        port: sshPort(),
        username: 'workbench-test',
        group: '开发环境',
      },
      async (dialog) => {
        await dialog.getByRole('button', { name: '密码', exact: true }).click()
      },
    )
    const saved = await (await page.request.get(`${baseURL}/api/v1/hosts`)).json()
    expect(saved).toHaveLength(1)
    expect(saved[0]).not.toHaveProperty('password')

    await page
      .locator('.host-sidebar')
      .getByTitle(`连接 workbench-test@127.0.0.1:${sshPort()}`, { exact: true })
      .click()
    await page.getByLabel('SSH 密码', { exact: true }).fill('local-fixture-only')
    await page.getByRole('button', { name: '连接主机', exact: true }).click()
    await expect(page.locator('.terminal-pane.active .terminal-toolbar')).toContainText('已连接')
    await page.getByRole('button', { name: '切换文件面板' }).click()
    await expect(page.locator('.file-list')).toContainText('欢迎.txt')
    terminalID = (await (await page.request.get(`${baseURL}/api/v1/terminals`)).json())[0].id
  })

  await test.step('Unicode travels through the real SSH and WebSocket byte path', async () => {
    const fontIsMonospaced = await page.locator('.xterm-rows').evaluate((element) => {
      const style = getComputedStyle(element)
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')!
      ctx.font = `${style.fontSize} ${style.fontFamily}`
      return Math.abs(ctx.measureText('M').width - ctx.measureText('i').width) < 0.1
    })
    expect(fontIsMonospaced).toBe(true)
    await page.locator('.terminal-pane.active .xterm-screen').click()
    await page.keyboard.insertText('echo 中文传输😀')
    await page.keyboard.press('Enter')
    await expect(page.locator('.terminal-pane.active .xterm-rows')).toContainText('中文传输')
    await page.keyboard.insertText('emit')
    await page.keyboard.press('Enter')
    await expect(page.locator('.terminal-pane.active .xterm-rows')).toContainText('分包中文')
    await page.screenshot({ path: join(screenshots, 'terminal-files.png') })
  })

  await test.step('editor loads only local assets and preserves BOM and CRLF', async () => {
    await page.locator('.file-row').filter({ hasText: '欢迎.txt' }).locator('.file-open').click()
    const editor = page.getByRole('region', { name: '远端文本编辑器' })
    await expect(editor.locator('.monaco-editor')).toBeVisible()
    await editor.locator('.monaco-editor').click({ position: { x: 180, y: 32 } })
    await page.keyboard.press('ControlOrMeta+A')
    await page.keyboard.insertText('已编辑中文内容\n第二行测试\n')
    await expect(editor.getByRole('button', { name: '保存', exact: true })).toBeEnabled()
    await editor.getByRole('button', { name: '保存', exact: true }).click()
    await expect(editor.locator('.editor-footer')).toContainText('已保存到远端')
    const content = await (
      await page.request.get(
        `${baseURL}/api/v1/terminals/${terminalID}/download?path=${encodeURIComponent('/workspace/欢迎.txt')}`,
      )
    ).body()
    expect(content.toString('utf8')).toBe('\ufeff已编辑中文内容\r\n第二行测试\r\n')
    await page.screenshot({ path: join(screenshots, 'editor.png') })
    const popupEvent = context.waitForEvent('page')
    await editor.getByRole('button', { name: '新窗口编辑' }).click()
    const popup = await popupEvent
    await expect(popup.locator('.monaco-editor')).toBeVisible()
    await expect(popup.locator('.editor-heading')).toContainText('欢迎.txt')
    await popup.close()
    await editor.getByRole('button', { name: '关闭编辑器' }).click()
  })

  await test.step('upload queue and native browser download preserve Chinese filenames', async () => {
    await page.getByLabel('上传文件', { exact: true }).setInputFiles({
      name: '上传 文档.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('上传内容：你好😀\n'),
    })
    const confirmation = page.getByRole('dialog', { name: '确认上传目标' })
    await expect(confirmation.getByLabel('远端目录（绝对路径）')).toHaveValue('/workspace')
    await confirmation.getByRole('button', { name: '确认上传', exact: true }).click()
    const transfer = page.locator('.transfer-row').filter({ hasText: '上传 文档.txt' })
    await expect(transfer).toContainText('已完成')
    const file = page.locator('.file-row').filter({ hasText: '上传 文档.txt' })
    await expect(file).toBeVisible()
    await file.hover()
    const downloadEvent = page.waitForEvent('download')
    await file.getByRole('link', { name: '下载 上传 文档.txt', exact: true }).click()
    const download = await downloadEvent
    expect(download.suggestedFilename()).toBe('上传 文档.txt')
    const destination = testInfo.outputPath('download.txt')
    await download.saveAs(destination)
    expect(await readFile(destination, 'utf8')).toBe('上传内容：你好😀\n')
    await page.screenshot({ path: join(screenshots, 'transfer.png') })
  })

  await test.step('folder upload keeps structure and folders download as zip', async () => {
    const folder = testInfo.outputPath('资料夹')
    await mkdir(join(folder, '子目录'), { recursive: true })
    await writeFile(join(folder, '说明.txt'), '顶层文件\n')
    await writeFile(join(folder, '子目录', '深层.txt'), '深层文件\n')
    await page.getByLabel('上传文件夹', { exact: true }).last().setInputFiles(folder)
    const confirmation = page.getByRole('dialog', { name: '确认上传目标' })
    await expect(confirmation).toContainText('1 个文件夹')
    await confirmation.getByRole('button', { name: '确认上传', exact: true }).click()
    await expect(
      page.locator('.transfer-row').filter({ hasText: '资料夹/子目录/深层.txt' }),
    ).toContainText('已完成')
    const row = page.locator('.file-row').filter({ hasText: '资料夹' })
    await expect(row).toBeVisible()
    await row.hover()
    const downloadEvent = page.waitForEvent('download')
    await row.getByRole('link', { name: '打包下载 资料夹', exact: true }).click()
    const download = await downloadEvent
    expect(download.suggestedFilename()).toBe('资料夹.zip')
    const archive = testInfo.outputPath('folder.zip')
    await download.saveAs(archive)
    const bytes = await readFile(archive)
    expect(bytes.subarray(0, 2).toString()).toBe('PK')
    expect(bytes.includes(Buffer.from('资料夹/子目录/深层.txt'))).toBe(true)
    await page.getByRole('button', { name: '收起传输任务' }).click()
  })

  await test.step('keyboard-interactive MFA through a single jump host', async () => {
    await createHost(
      page,
      { name: 'MFA 目标', port: ssh2Port(), username: 'workbench-mfa' },
      async (dialog) => {
        await dialog.getByRole('button', { name: 'MFA', exact: true }).click()
        await dialog
          .getByLabel('跳板机（ProxyJump，单级）')
          .selectOption({ label: `中文验收主机 · workbench-test@127.0.0.1:${sshPort()}` })
        await dialog.getByLabel('生产主机（醒目标记）').check()
      },
    )
    await page
      .locator('.host-sidebar')
      .getByTitle(`连接 workbench-mfa@127.0.0.1:${ssh2Port()}`, { exact: true })
      .click()
    // Jump host needs password; prompt opens cleanly, then TOFU trusts target and shows MFA challenge.
    const connectDialog = page.getByRole('dialog', { name: '连接 MFA 目标' })
    await expect(connectDialog).toContainText('经由 中文验收主机')
    await connectDialog.getByLabel('SSH 密码', { exact: true }).fill('local-fixture-only')
    await connectDialog.getByRole('button', { name: '连接主机', exact: true }).click()
    const challenge = page.getByRole('dialog', { name: '服务器认证提问' })
    await expect(challenge).toBeVisible()
    await expect(challenge.getByLabel('Password:')).toHaveAttribute('type', 'password')
    await challenge.getByLabel('Password:').fill('local-mfa-only')
    await challenge.getByLabel('Verification code:').fill('246810')
    await challenge.getByRole('button', { name: '提交回答' }).click()
    await expect(challenge).not.toBeVisible()
    await expect(page.locator('.terminal-pane.active .terminal-toolbar')).toContainText('已连接')
    await expect(page.locator('.terminal-pane.active .production-badge')).toBeVisible()
    await expect(page.locator('.file-list')).toContainText('欢迎.txt')
  })

  await test.step('split panes show both sessions and broadcast sends one whole command to each', async () => {
    await page.getByRole('button', { name: '左右分屏', exact: true }).click()
    await expect(page.locator('.pane-cell .terminal-pane')).toHaveCount(2)
    await page.screenshot({ path: join(screenshots, 'split.png') })
    await page.getByRole('button', { name: '多主机广播', exact: true }).click()
    const panel = page.getByRole('dialog', { name: '命令片段与广播' })
    await panel.getByLabel('命令', { exact: true }).fill('echo 广播测试')
    await panel.getByLabel(/中文验收主机/).check()
    await panel.getByLabel(/MFA 目标/).check()
    await panel.getByRole('button', { name: '预览' }).click()
    await panel.getByLabel('请输入目标主机数量 2 以确认').fill('2')
    await panel.getByRole('button', { name: '发送到 2 台主机' }).click()
    await expect(panel).toContainText('已发送到 2 台')
    await panel.getByRole('button', { name: '关闭', exact: true }).first().click()
    for (const pane of await page.locator('.pane-cell .terminal-pane').all())
      await expect(pane.locator('.xterm-rows')).toContainText('广播测试')
  })

  await test.step('synthetic IME composition after switching tabs commits once (not a real IME test)', async () => {
    const tabs = page.locator('.session-tab .tab-select')
    await tabs.nth(1).click()
    await tabs.nth(0).click()
    const pane = page.locator('.terminal-pane.active')
    await pane.locator('.xterm-screen').click()
    await page.bringToFront()
    const cdp = await context.newCDPSession(page)
    await page.keyboard.type('echo ime-')
    await expect(pane.locator('.xterm-rows')).toContainText('$ echo ime-')
    await cdp.send('Input.imeSetComposition', { text: 'zhong', selectionStart: 5, selectionEnd: 5 })
    await cdp.send('Input.imeSetComposition', { text: '中文', selectionStart: 2, selectionEnd: 2 })
    await cdp.send('Input.insertText', { text: '中文输入法' })
    await page.keyboard.press('Enter')
    await expect(pane.locator('.xterm-rows')).toContainText('echo ime-中文输入法')
    await expect
      .poll(
        async () => (await pane.locator('.xterm-rows').innerText()).split('中文输入法').length - 1,
      )
      .toBe(2)
    expect(await pane.locator('.xterm-rows').innerText()).not.toContain('zhong')
    await cdp.detach()
  })

  await test.step('configurable shortcuts switch layouts and are not typed into the shell', async () => {
    await page.locator('.terminal-pane.active .xterm-screen').click()
    await page.keyboard.press('Alt+Shift+Digit1')
    await expect(page.locator('.pane-single .terminal-pane')).toHaveCount(1)
    await expect(page.locator('.pane-cell')).toHaveCount(0)
    await page.keyboard.press('Alt+Shift+Digit4')
    await expect(page.locator('.pane-cell')).toHaveCount(6)
    await page.keyboard.press('Alt+Shift+Digit1')
  })

  await test.step('local port forwarding runs independently of terminals', async () => {
    const port = 41000 + Math.floor(Math.random() * 2000)
    await page.getByRole('button', { name: '显示端口转发隧道' }).click()
    const drawer = page.getByRole('region', { name: '端口转发隧道' })
    await expect(drawer).toContainText('不对外公开')
    await drawer.getByRole('button', { name: '新建隧道' }).click()
    const dialog = page.getByRole('dialog', { name: '新建隧道' })
    await dialog.getByLabel('名称').fill('回声服务')
    await dialog
      .getByLabel('SSH 主机')
      .selectOption({ label: `中文验收主机 · workbench-test@127.0.0.1:${sshPort()}` })
    await dialog.getByLabel('本地端口').fill(String(port))
    await dialog.getByLabel('目标主机').fill('127.0.0.1')
    await dialog.getByLabel('目标端口').fill(echoPort())
    await dialog.getByRole('button', { name: '创建隧道' }).click()
    await expect(dialog).not.toBeVisible()
    await drawer.getByRole('button', { name: '启动隧道 回声服务' }).click()
    const start = page.getByRole('dialog', { name: '启动隧道 回声服务' })
    await start.getByLabel('SSH 密码', { exact: true }).fill('local-fixture-only')
    await start.getByRole('button', { name: '启动隧道', exact: true }).click()
    await expect(start).not.toBeVisible()
    await expect(drawer).toContainText('运行中')
    expect(await echoThrough(port, '隧道往返 tunnel')).toBe('隧道往返 tunnel')
    await page.screenshot({ path: join(screenshots, 'tunnels.png') })
    await drawer.getByRole('button', { name: '停止隧道 回声服务' }).click()
    await expect(drawer).toContainText('已停止')
    await drawer.getByRole('button', { name: '收起端口转发隧道' }).click()
  })

  await test.step('CLI edit opens a local file directly without token gate', async () => {
    const document = testInfo.outputPath('本机配置.conf')
    await writeFile(document, 'key = 旧值\n')
    const output = execFileSync(
      process.env.SSHWB_BINARY!,
      ['edit', '--data-dir', process.env.SSHWB_DATA_DIR!, '--no-browser', document],
      { encoding: 'utf8', env: { ...process.env, HOME: process.env.SSHWB_HOME } },
    )
    const url = output.match(/Open: (\S+)/)![1]
    const editorPage = await context.newPage()
    await editorPage.goto(url)
    expect(new URL(editorPage.url()).hash).toBe('')
    const editor = editorPage.getByRole('region', { name: '远端文本编辑器' })
    await expect(editor.locator('.monaco-editor')).toBeVisible()
    await editor.locator('.monaco-editor').click({ position: { x: 180, y: 20 } })
    await editorPage.keyboard.press('ControlOrMeta+A')
    await editorPage.keyboard.insertText('key = 新值\n')
    await editor.getByRole('button', { name: '保存', exact: true }).click()
    await expect(editor.locator('.editor-footer')).toContainText('已保存到本机文件')
    expect(await readFile(document, 'utf8')).toBe('key = 新值\n')
    await editorPage.close()
  })

  await test.step('reload restores the layout as placeholders without auth lock', async () => {
    await expect
      .poll(
        async () =>
          (await (await page.request.get(`${baseURL}/api/v1/workspace`)).json()).layout.tabs.length,
      )
      .toBe(2)
    await page.reload()
    await expect(page.locator('.session-tab')).toHaveCount(2)
    await expect(page.getByText('中文验收主机 · 未连接')).toBeVisible()
    await expect(page.getByRole('button', { name: '重新连接' })).toBeVisible()
    await expect(page.locator('.workspace-warning')).toContainText('旧会话')
    await page.getByRole('button', { name: '立即清理' }).click()
  })

  await test.step('vault saves credentials only after explicit choice and a successful login', async () => {
    await page.getByRole('button', { name: '工作区设置', exact: true }).click()
    const settings = page.getByRole('dialog', { name: '设置' })
    await settings.getByRole('tab', { name: '凭据库' }).click()
    await settings.getByRole('radio', { name: /主密码/ }).check()
    await settings.getByLabel('主密码', { exact: true }).fill('端到端测试主密码 e2e')
    await settings.getByLabel('再次输入主密码').fill('端到端测试主密码 e2e')
    await settings.getByRole('button', { name: '启用凭据库' }).click()
    await expect(settings).toContainText('已解锁')
    await settings.getByRole('button', { name: '关闭对话框' }).click()
    await page.getByText('中文验收主机 · 未连接').click()
    await page.getByRole('button', { name: '重新连接' }).first().click()
    const connectDialog = page.getByRole('dialog', { name: '连接 中文验收主机' })
    await connectDialog.getByLabel('SSH 密码', { exact: true }).fill('local-fixture-only')
    await connectDialog.getByLabel('连接成功后加密保存此凭据并关联到主机').check()
    await connectDialog.getByRole('button', { name: '连接主机', exact: true }).click()
    await expect(connectDialog).not.toBeVisible()
    await expect(page.locator('.terminal-pane.active .terminal-toolbar')).toContainText('已连接')
    const records = await (await page.request.get(`${baseURL}/api/v1/credentials`)).json()
    expect(records).toHaveLength(1)
    expect(JSON.stringify(records)).not.toContain('local-fixture-only')
    const hosts = await (await page.request.get(`${baseURL}/api/v1/hosts`)).json()
    expect(hosts.find((host: { name: string }) => host.name === '中文验收主机').credentialId).toBe(
      records[0].id,
    )
  })

  await test.step('OpenSSH config import supports ProxyCommand and reuses existing jumps', async () => {
    await page.getByRole('button', { name: '导入 OpenSSH 配置' }).click()
    const dialog = page.getByRole('dialog', { name: '导入 SSH 配置' })
    await dialog.getByRole('button', { name: '粘贴配置内容' }).click()
    await dialog
      .getByLabel('配置内容')
      .fill(
        [
          'Host bastion-e2e',
          '  HostName 127.0.0.1',
          `  Port ${sshPort()}`,
          '  User workbench-test',
          'Host app-e2e',
          '  HostName 10.20.30.40',
          '  User deploy',
          '  ProxyJump bastion-e2e',
          'Host proxy-cmd-e2e',
          '  HostName 10.20.30.41',
          '  User ops',
          '  ProxyCommand nc %h %p',
          'Match exec "touch /tmp/sshwb-e2e-should-not-exist"',
          '  User nobody',
        ].join('\n'),
      )
    await dialog.getByRole('button', { name: /解析/ }).click()
    await expect(dialog).toContainText('app-e2e')
    await expect(dialog).toContainText('proxy-cmd-e2e')
    await dialog.getByLabel('选择 app-e2e').check()
    await dialog.getByLabel('选择 proxy-cmd-e2e').check()
    await dialog.getByRole('button', { name: /导入所选/ }).click()
    await expect(dialog).toContainText('bastion-e2e')
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(page.locator('.host-sidebar')).toContainText('app-e2e')
    await expect(page.locator('.host-sidebar')).toContainText('proxy-cmd-e2e')
    const imported = await (await page.request.get(`${baseURL}/api/v1/hosts`)).json()
    const app = imported.find((host: { name: string }) => host.name === 'app-e2e')
    const proxyHost = imported.find((host: { name: string }) => host.name === 'proxy-cmd-e2e')
    const existing = imported.find((host: { name: string }) => host.name === '中文验收主机')
    expect(app.jumpHostId).toBe(existing.id)
    expect(proxyHost.proxyCommand).toBe('nc %h %p')
    expect(imported.some((host: { name: string }) => host.name === 'bastion-e2e')).toBe(false)
    expect(browserErrors).toEqual([])
  })
})
