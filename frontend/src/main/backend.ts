import { execFileSync, type ChildProcess, spawn } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { delimiter, join, resolve } from 'node:path'
import { app, powerSaveBlocker } from 'electron'

// Single source of truth for where the spawned backend lives; returned to the
// renderer through the IPC result so both sides always agree.
const LOCAL_BACKEND_URL = 'http://127.0.0.1:5299'
const LOCAL_HEALTH_URL = `${LOCAL_BACKEND_URL}/health`
const START_TIMEOUT_MS = 30_000

let child: ChildProcess | null = null
let exitError: string | null = null
let stderrTail = ''
let stdoutBuffer = ''
let startupKey = ''
let startupRoot = ''
let powerSaveBlockerID: number | null = null

type HealthStatus = {
  ok: boolean
  authRequired: boolean
}

type LocalBackendResult = {
  ok: boolean
  url?: string
  key?: string
  error?: string
}

type TerminateOptions = {
  timeoutMs?: number
  forceTimeoutMs?: number
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const pidFilePath = (): string => join(app.getPath('userData'), 'local-backend.pid')
const defaultRootPath = (): string => join(homedir(), '.jaz')
const authFilePath = (root = defaultRootPath()): string => join(root, 'auth.json')
const backendBinaryName = (): string => (process.platform === 'win32' ? 'jaz.exe' : 'jaz')

function localBackendEnv(extraPath?: string): NodeJS.ProcessEnv {
  const env = { ...process.env }
  if (process.platform === 'darwin') {
    const shell = env['SHELL']?.trim() || '/bin/zsh'
    try {
      const out = execFileSync(shell, ['-l', '-i', '-c', 'printf "\\n__JAZ_PATH__%s\\n" "$PATH"'], {
        encoding: 'utf8',
        timeout: 3_000,
        stdio: ['ignore', 'pipe', 'ignore'],
      })
      let line = ''
      for (const entry of out.split(/\r?\n/).reverse()) {
        if (!entry.startsWith('__JAZ_PATH__')) continue
        line = entry.slice('__JAZ_PATH__'.length).trim()
        break
      }
      if (line) env['PATH'] = line
    } catch {
      // Keep LaunchServices env.
    }
  }
  if (extraPath) env['PATH'] = [extraPath, env['PATH'] ?? env['Path'] ?? ''].filter(Boolean).join(delimiter)
  return env
}

// The pid file marks a backend WE spawned. If it survives into a new session
// (dev-watcher restart, crash — before-quit never ran), kill that group before
// probing health, or we'd silently adopt a server running stale code. Backends
// the user started themselves have no pid file and are still adopted.
async function reapOrphan(): Promise<void> {
  let rawPid: string
  try {
    rawPid = readFileSync(pidFilePath(), 'utf8')
  } catch {
    return
  }
  try {
    rmSync(pidFilePath())
  } catch {
    // best effort
  }
  const pid = Number(rawPid.trim())
  if (!Number.isInteger(pid) || pid <= 1) return
  if (!backendPIDAlive(pid)) return
  signalBackendPID(pid, 'SIGTERM')
  // wait for the group to release the port before we spawn a fresh one
  const deadline = Date.now() + 3_000
  while (Date.now() < deadline) {
    if (!backendPIDAlive(pid)) {
      return
    }
    await sleep(150)
  }
  if (process.platform === 'win32') killWindowsTree(pid, true)
}

function readAuthKey(path: string): string {
  try {
    const body = JSON.parse(readFileSync(path, 'utf8')) as { api_key?: unknown }
    return typeof body.api_key === 'string' ? body.api_key.trim() : ''
  } catch {
    return ''
  }
}

function readLocalAuthKey(): string {
  if (startupKey) return startupKey
  if (startupRoot) return readAuthKey(authFilePath(startupRoot))
  return readAuthKey(authFilePath())
}

function startPowerSaveBlocker(): void {
  if (powerSaveBlockerID !== null && powerSaveBlocker.isStarted(powerSaveBlockerID)) return
  powerSaveBlockerID = powerSaveBlocker.start('prevent-app-suspension')
}

function stopPowerSaveBlocker(): void {
  if (powerSaveBlockerID === null) return
  if (powerSaveBlocker.isStarted(powerSaveBlockerID)) {
    powerSaveBlocker.stop(powerSaveBlockerID)
  }
  powerSaveBlockerID = null
}

async function localBackendStartResult(
  health: HealthStatus,
  source: 'adopted' | 'spawned',
): Promise<LocalBackendResult> {
  const result = await localBackendResult(health, source)
  if (result.ok && source === 'spawned') startPowerSaveBlocker()
  return result
}

function captureStartupLine(line: string): void {
  const text = line.trim()
  const client = text.match(/^client:\s+(.+)$/)
  if (client) {
    try {
      startupKey = new URL(client[1]).searchParams.get('key')?.trim() ?? startupKey
    } catch {
      // ignored; the auth file fallback still applies
    }
    return
  }
  const root = text.match(/^root:\s+(.+)$/)
  if (root) startupRoot = root[1].trim()
}

function captureStartupOutput(chunk: Buffer): void {
  stdoutBuffer += chunk.toString()
  const lines = stdoutBuffer.split(/\r?\n/)
  stdoutBuffer = lines.pop() ?? ''
  for (const line of lines) captureStartupLine(line)
}

async function readHealth(): Promise<HealthStatus | null> {
  try {
    const res = await fetch(LOCAL_HEALTH_URL, { signal: AbortSignal.timeout(2_000) })
    if (!res.ok) return null
    const body = (await res.json()) as { ok?: boolean; auth_required?: boolean }
    return { ok: body.ok === true, authRequired: body.auth_required === true }
  } catch {
    return null
  }
}

async function localBackendResult(
  health: HealthStatus,
  source: 'adopted' | 'spawned',
): Promise<LocalBackendResult> {
  const key = readLocalAuthKey()
  if (health.authRequired && !key) {
    if (source === 'adopted') {
      return { ok: true, url: LOCAL_BACKEND_URL }
    }
    return {
      ok: false,
      error: `Jaz started the backend, but its auth key was not printed and was not readable at ${authFilePath(startupRoot || defaultRootPath())}.`,
    }
  }
  return { ok: true, url: LOCAL_BACKEND_URL, key: key || undefined }
}

function spawnBackend(): ChildProcess {
  if (app.isPackaged) {
    // Run from the runtime root so application.yaml and .env can live next to
    // the server's data.
    const binDir = join(process.resourcesPath, 'bin')
    const bin = join(binDir, backendBinaryName())
    const cwd = join(homedir(), '.jaz')
    mkdirSync(cwd, { recursive: true })
    return spawn(bin, [], {
      cwd,
      detached: true,
      env: localBackendEnv(binDir),
      stdio: ['ignore', 'pipe', 'pipe'],
    })
  }
  // Dev: out/main → frontend/out/main, so the backend module sits three up.
  const backendDir = resolve(__dirname, '../../../backend')
  return spawn('go', ['run', './cmd/jaz'], {
    cwd: backendDir,
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

function signalBackendPID(pid: number, signal: NodeJS.Signals): void {
  if (process.platform === 'win32') {
    killWindowsTree(pid, signal === 'SIGKILL')
    return
  }
  signalPOSIXProcessGroup(pid, signal)
}

function signalPOSIXProcessGroup(pid: number, signal: NodeJS.Signals): boolean {
  try {
    process.kill(-pid, signal)
    return true
  } catch {
    return false
  }
}

function backendPIDAlive(pid: number): boolean {
  try {
    process.kill(process.platform === 'win32' ? pid : -pid, 0)
    return true
  } catch {
    return false
  }
}

function killWindowsTree(pid: number, force: boolean): void {
  const args = ['/pid', String(pid), '/t']
  if (force) args.push('/f')
  const killer = spawn('taskkill', args, {
    stdio: 'ignore',
    windowsHide: true,
  })
  killer.on('error', () => {
    try {
      process.kill(pid, force ? 'SIGKILL' : 'SIGTERM')
    } catch {
      // already gone
    }
  })
}

function signalBackendProcess(proc: ChildProcess, signal: NodeJS.Signals): void {
  if (!proc?.pid) return
  if (process.platform === 'win32') {
    killWindowsTree(proc.pid, signal === 'SIGKILL')
    return
  }
  if (signalPOSIXProcessGroup(proc.pid, signal)) return
  try {
    proc.kill(signal)
  } catch {
    // already gone
  }
}

function takeLocalBackend(): ChildProcess | null {
  const proc = child
  child = null
  try {
    rmSync(pidFilePath())
  } catch {
    // nothing spawned this session
  }
  return proc?.pid ? proc : null
}

function waitForExit(proc: ChildProcess, timeoutMs: number): Promise<boolean> {
  if (proc.exitCode !== null || proc.signalCode !== null) return Promise.resolve(true)
  return new Promise((resolve) => {
    let settled = false
    const done = (exited: boolean): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      proc.off('exit', onExit)
      resolve(exited)
    }
    const onExit = (): void => done(true)
    const timer = setTimeout(() => done(false), timeoutMs)
    timer.unref()
    proc.once('exit', onExit)
  })
}

export async function startLocalBackend(): Promise<LocalBackendResult> {
  if (!child) await reapOrphan()
  let health = await readHealth()
  if (health?.ok) return localBackendStartResult(health, child ? 'spawned' : 'adopted')

  if (!child) {
    exitError = null
    stderrTail = ''
    stdoutBuffer = ''
    startupKey = ''
    startupRoot = ''
    let proc: ChildProcess
    try {
      proc = spawnBackend()
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : String(err) }
    }
    proc.stdout?.on('data', (chunk: Buffer) => {
      captureStartupOutput(chunk)
      process.stdout.write(`[jaz] ${chunk}`)
    })
    proc.stderr?.on('data', (chunk: Buffer) => {
      process.stderr.write(`[jaz] ${chunk}`)
      stderrTail = (stderrTail + chunk.toString()).slice(-2_000)
    })
    proc.on('error', (err) => {
      exitError = err.message
      stopPowerSaveBlocker()
      if (child === proc) child = null
    })
    proc.on('exit', (code, signal) => {
      // a nonzero exit usually leaves the real reason on stderr; signal kills
      // and clean exits get a plain message instead of "code null"
      const detail = stderrTail.trim().split('\n').at(-1)
      exitError = code
        ? detail || `backend exited with code ${code}`
        : `backend exited${signal ? ` (${signal})` : ''}`
      try {
        rmSync(pidFilePath())
      } catch {
        // already gone
      }
      stopPowerSaveBlocker()
      if (child === proc) child = null
    })
    if (proc.pid) {
      try {
        writeFileSync(pidFilePath(), String(proc.pid))
      } catch {
        // pid file is best effort; worst case a future session adopts this one
      }
    }
    child = proc
  }

  const deadline = Date.now() + START_TIMEOUT_MS
  let authError: string | null = null
  while (Date.now() < deadline) {
    health = await readHealth()
    if (health?.ok) {
      const result = await localBackendStartResult(health, 'spawned')
      if (result.ok) return result
      authError = result.error ?? null
    }
    if (!child) return { ok: false, error: exitError ?? 'backend exited' }
    await new Promise((r) => setTimeout(r, 500))
  }
  if (authError) return { ok: false, error: authError }
  return { ok: false, error: 'timed out waiting for the backend to become healthy' }
}

export function stopLocalBackend(): void {
  stopPowerSaveBlocker()
  const proc = takeLocalBackend()
  if (proc) signalBackendProcess(proc, 'SIGTERM')
}

export async function terminateLocalBackend(options: TerminateOptions = {}): Promise<void> {
  stopPowerSaveBlocker()
  const proc = takeLocalBackend()
  if (!proc) return
  signalBackendProcess(proc, 'SIGTERM')
  const timeoutMs = options.timeoutMs ?? 0
  if (timeoutMs <= 0 || (await waitForExit(proc, timeoutMs))) return
  signalBackendProcess(proc, 'SIGKILL')
  const forceTimeoutMs = options.forceTimeoutMs ?? 0
  if (forceTimeoutMs > 0) await waitForExit(proc, forceTimeoutMs)
}
