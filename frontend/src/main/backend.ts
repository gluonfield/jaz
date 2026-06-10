import { type ChildProcess, spawn } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import { app } from 'electron'

// The renderer owns the backend URL, but "Start locally" always targets the
// default local port the bundled `jaz serve` binds to.
const LOCAL_HEALTH_URL = 'http://127.0.0.1:8080/health'
const START_TIMEOUT_MS = 30_000

let child: ChildProcess | null = null
let exitError: string | null = null
let stderrTail = ''

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

export async function startLocalBackend(): Promise<{ ok: boolean; error?: string }> {
  if (await isHealthy()) return { ok: true }

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
    proc.on('exit', (code) => {
      exitError = stderrTail.trim().split('\n').at(-1) || `backend exited with code ${code}`
      if (child === proc) child = null
    })
    child = proc
  }

  const deadline = Date.now() + START_TIMEOUT_MS
  while (Date.now() < deadline) {
    if (await isHealthy()) return { ok: true }
    if (!child) return { ok: false, error: exitError ?? 'backend exited' }
    await new Promise((r) => setTimeout(r, 500))
  }
  return { ok: false, error: 'timed out waiting for the backend to become healthy' }
}

export function stopLocalBackend(): void {
  const proc = child
  child = null
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
