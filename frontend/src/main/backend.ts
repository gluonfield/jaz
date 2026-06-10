import { type ChildProcess, spawn } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import { app } from 'electron'

// Single source of truth for where the spawned backend lives; returned to the
// renderer through the IPC result so both sides always agree.
const LOCAL_BACKEND_URL = 'http://127.0.0.1:8080'
const LOCAL_HEALTH_URL = `${LOCAL_BACKEND_URL}/health`
const START_TIMEOUT_MS = 30_000

let child: ChildProcess | null = null
let exitError: string | null = null
let stderrTail = ''

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const pidFilePath = (): string => join(app.getPath('userData'), 'local-backend.pid')

// The pid file marks a backend WE spawned. If it survives into a new session
// (dev-watcher restart, crash — before-quit never ran), kill that group before
// probing health, or we'd silently adopt a server running stale code. Backends
// the user started themselves have no pid file and are still adopted.
async function reapOrphan(): Promise<void> {
  let pid = 0
  try {
    pid = Number(readFileSync(pidFilePath(), 'utf8').trim())
  } catch {
    return
  }
  try {
    rmSync(pidFilePath())
  } catch {
    // best effort
  }
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

async function isHealthy(): Promise<boolean> {
  try {
    const res = await fetch(LOCAL_HEALTH_URL, { signal: AbortSignal.timeout(2_000) })
    if (!res.ok) return false
    const body = (await res.json()) as { ok?: boolean }
    return body.ok === true
  } catch {
    return false
  }
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

export async function startLocalBackend(): Promise<{ ok: boolean; url?: string; error?: string }> {
  if (!child) await reapOrphan()
  if (await isHealthy()) return { ok: true, url: LOCAL_BACKEND_URL }

  if (!child) {
    exitError = null
    stderrTail = ''
    let proc: ChildProcess
    try {
      proc = spawnBackend()
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : String(err) }
    }
    proc.stdout?.on('data', (chunk: Buffer) => process.stdout.write(`[jaz] ${chunk}`))
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
  while (Date.now() < deadline) {
    if (await isHealthy()) return { ok: true, url: LOCAL_BACKEND_URL }
    if (!child) return { ok: false, error: exitError ?? 'backend exited' }
    await new Promise((r) => setTimeout(r, 500))
  }
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
