import { execFileSync, spawn, type ChildProcess } from 'node:child_process'
import { copyFile, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

async function launch(
  command: string,
  args: string[],
  pattern: RegExp,
  children: ChildProcess[],
  environment: NodeJS.ProcessEnv = process.env,
): Promise<{ match: string; output: string }> {
  const child = spawn(command, args, {
    cwd: root,
    stdio: ['ignore', 'pipe', 'pipe'],
    env: environment,
  })
  children.push(child)
  return new Promise((resolveReady, reject) => {
    let output = ''
    const timer = setTimeout(
      () =>
        reject(
          new Error(
            `Fixture startup timed out: ${output.replace(/#bootstrap=\S+/g, '#bootstrap=REDACTED')}`,
          ),
        ),
      45000,
    )
    const inspect = (data: Buffer) => {
      output += data.toString()
      const match = output.match(pattern)
      if (match) {
        clearTimeout(timer)
        resolveReady({ match: match[1], output })
      }
    }
    child.stdout?.on('data', inspect)
    child.stderr?.on('data', inspect)
    child.once('error', (error) => {
      clearTimeout(timer)
      reject(error)
    })
    child.once('exit', (code) => {
      clearTimeout(timer)
      reject(
        new Error(
          `Fixture exited (${code}): ${output.replace(/#bootstrap=\S+/g, '#bootstrap=REDACTED')}`,
        ),
      )
    })
  })
}

async function stop(children: ChildProcess[]) {
  await Promise.all(
    children.map(
      (child) =>
        new Promise<void>((resolveStopped) => {
          if (child.exitCode !== null) {
            resolveStopped()
            return
          }
          const timer = setTimeout(() => child.kill('SIGKILL'), 5000)
          child.once('exit', () => {
            clearTimeout(timer)
            resolveStopped()
          })
          child.kill('SIGINT')
        }),
    ),
  )
}

export default async function setupE2E() {
  const scratch = await mkdtemp(join(tmpdir(), 'ssh-workbench-e2e-'))
  const children: ChildProcess[] = []
  const suffix = process.platform === 'win32' ? '.exe' : ''
  try {
    await mkdir(join(root, 'bin'), { recursive: true })
    await mkdir(join(root, '.local/screenshots'), { recursive: true })
    execFileSync('go', ['build', '-o', `bin/sshw${suffix}`, './cmd/sshw'], {
      cwd: root,
      stdio: 'pipe',
      timeout: 180000,
    })
    execFileSync('go', ['build', '-o', `bin/fixture-sshd${suffix}`, './tests/fixtures/sshd'], {
      cwd: root,
      stdio: 'pipe',
      timeout: 180000,
    })
    const keyPath = join(scratch, 'client-key')
    const primary = await launch(
      join(root, `bin/fixture-sshd${suffix}`),
      ['--listen', '127.0.0.1:0', '--echo-listen', '127.0.0.1:0', '--client-key-out', keyPath],
      /Fixture SSH: (\S+)[\s\S]*In-memory filesystem/,
      children,
    )
    const secondary = await launch(
      join(root, `bin/fixture-sshd${suffix}`),
      ['--listen', '127.0.0.1:0'],
      /Fixture SSH: (\S+)/,
      children,
    )
    process.env.SSHWB_SSH_PORT = primary.match.split(':').at(-1)
    process.env.SSHWB_SSH2_PORT = secondary.match.split(':').at(-1)
    process.env.SSHWB_ECHO_PORT = primary.output
      .match(/Fixture echo: (\S+)/)?.[1]
      .split(':')
      .at(-1)
    process.env.SSHWB_KEY_PATH = keyPath
    // The server runs with a private HOME so it never reads the developer's
    // ~/.ssh (config, known_hosts, keys). The vault is exercised in master
    // password mode only, so the OS keychain is never touched.
    const home = join(scratch, 'home')
    await mkdir(join(home, '.ssh'), { recursive: true, mode: 0o700 })
    await copyFile(keyPath, join(home, '.ssh', 'id_ed25519'))
    await writeFile(
      join(home, '.ssh', 'config'),
      `Host fixture-mac-config\n  HostName 127.0.0.1\n  Port ${process.env.SSHWB_SSH_PORT}\n  User workbench-key\n`,
      { mode: 0o600 },
    )
    const dataDirectory = join(scratch, 'private')
    process.env.SSHWB_HOME = home
    process.env.SSHWB_DATA_DIR = dataDirectory
    process.env.SSHWB_BINARY = join(root, `bin/sshw${suffix}`)
    const started = await launch(
      join(root, `bin/sshw${suffix}`),
      ['--listen', '127.0.0.1:0', '--data-dir', dataDirectory, '--no-browser'],
      /Open: (\S+)/,
      children,
      { ...process.env, HOME: home, USERPROFILE: home, SSH_AUTH_SOCK: '' },
    )
    process.env.SSHWB_BOOTSTRAP_URL = started.match
    process.env.SSHWB_SCREENSHOTS = join(root, '.local/screenshots')
    return async () => {
      await stop(children)
      await rm(scratch, { recursive: true, force: true })
    }
  } catch (error) {
    await stop(children)
    await rm(scratch, { recursive: true, force: true })
    throw error
  }
}
