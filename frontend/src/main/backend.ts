import { type ChildProcess, spawn } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import { app } from 'electron'

// Single source of truth for where the spawned backend lives; returned to the
// renderer through the IPC result so both sides always agree.
const LOCAL_BACKEND_URL = 'http://127.0.0.1:5299'
const LOCAL_HEALTH_URL = `${LOCAL_BACKEND_URL}/health`
const LOCAL_AUTH_CHECK_URL = `${LOCAL_BACKEND_URL}/v1/auth/check`
const START_TIMEOUT_MS = 30_000

let child: ChildProcess | null = null
let exitError: string | null = null
let stderrTail = ''
let stdoutBuffer = ''
let startupKey = ''
let startupRoot = ''

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

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const pidFilePath = (): string => join(app.getPath('userData'), 'local-backend.pid')
const defaultRootPath = (): string => join(homedir(), '.jaz')
const authFilePath = (root = defaultRootPath()): string => join(root, 'auth.json')

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
  try {
    process.kill(-pid, 'SIGTERM')
  } catch {
    return // already gone
  }
  // wait for the group to release the port before we spawn a fresh one
  const deadline = Date.now() + 3_000
  while (Date.now() < deadline) {
    try {
      process.kill(-pid, 0)
    } catch {
      return
    }
    await sleep(150)
  }
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

async function checkAuthKey(key: string): Promise<boolean> {
  try {
    const res = await fetch(LOCAL_AUTH_CHECK_URL, {
      headers: { Authorization: `Bearer ${key}` },
      signal: AbortSignal.timeout(2_000),
    })
    return res.ok
  } catch {
    return false
  }
}

async function localBackendResult(
  health: HealthStatus,
  source: 'adopted' | 'spawned',
): Promise<LocalBackendResult> {
  const key = readLocalAuthKey()
  if (health.authRequired && !key) {
    if (source === 'adopted') {
      return {
        ok: false,
        error: `A backend is already running at ${LOCAL_BACKEND_URL}, but its key is not available at ${authFilePath()}. Paste its client URL or stop that backend and start locally.`,
      }
    }
    return {
      ok: false,
      error: `Jaz started the backend, but its auth key was not printed and was not readable at ${authFilePath(startupRoot || defaultRootPath())}.`,
    }
  }
  if (health.authRequired && !(await checkAuthKey(key))) {
    return {
      ok: false,
      error:
        source === 'adopted'
          ? `A backend is already running at ${LOCAL_BACKEND_URL}, but the key in ${authFilePath(startupRoot || defaultRootPath())} was rejected. Paste its client URL or stop that backend and start locally.`
          : 'Jaz started the backend, but the captured auth key was rejected.',
    }
  }
  return { ok: true, url: LOCAL_BACKEND_URL, key: key || undefined }
}

function spawnBackend(): ChildProcess {
  if (app.isPackaged) {
    // The Go binary ships as an extraResource; run it from ~/.jaz so the
    // server picks up application.yaml / .env dropped next to its data root.
    const bin = join(process.resourcesPath, 'bin', 'jaz')
    const cwd = join(homedir(), '.jaz')
    mkdirSync(cwd, { recursive: true })
    return spawn(bin, ['serve'], { cwd, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  }
  // Dev: out/main → frontend/out/main, so the backend module sits three up.
  const backendDir = resolve(__dirname, '../../../backend')
  return spawn('go', ['run', './cmd/jaz', 'serve'], {
    cwd: backendDir,
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

export async function startLocalBackend(): Promise<LocalBackendResult> {
  if (!child) await reapOrphan()
  let health = await readHealth()
  if (health?.ok) return localBackendResult(health, child ? 'spawned' : 'adopted')

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
      const result = await localBackendResult(health, 'spawned')
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
  const proc = child
  child = null
  try {
    rmSync(pidFilePath())
  } catch {
    // nothing spawned this session
  }
  if (!proc?.pid) return
  // Detached spawn puts `go run` and its server child in one group; negative
  // pid signals the whole group so the grandchild doesn't outlive the app.
  try {
    process.kill(-proc.pid, 'SIGTERM')
  } catch {
    try {
      proc.kill('SIGTERM')
    } catch {
      // already gone
    }
  }
}
