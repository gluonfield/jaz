import { afterAll, beforeEach, expect, mock, test } from 'bun:test'
import { mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

if (process.env.JAZ_FILE_ACTIONS_TEST_CHILD === '1') {
  const runs = fileURLToPath(new URL('../../../runs/', import.meta.url))
  mkdirSync(runs, { recursive: true })
  const directory = mkdtempSync(join(runs, 'file-actions-test-'))
  const target = join(directory, 'report.md')
  writeFileSync(target, '# Report')
  const handlers = new Map()
  const revealed = []
  const requests = []
  let probe = 'local'
  let replaceConnection = false
  let redirects = 0
  const other = Bun.serve({ port: 0, fetch: () => {
    redirects += 1
    return new Response('unexpected')
  } })
  const backend = Bun.serve({ port: 0, async fetch(request) {
    requests.push(request)
    if (request.headers.get('Authorization') !== 'Bearer device-token') return new Response(null, { status: 401 })
    const url = new URL(request.url)
    if (url.pathname !== '/v1/sessions/thread/file') return new Response(null, { status: 404 })
    if (replaceConnection === true || (replaceConnection === 'file' && !url.searchParams.has('raw'))) active = { ...active, revision: active.revision + 1 }
    const path = url.searchParams.get('path')
    if (url.searchParams.has('raw')) {
      if (probe === 'redirect') return Response.redirect(other.url)
      return new Response(probe === 'local' ? await readFile(path) : 'another machine')
    }
    return Response.json({ path: target })
  } })
  let active
  mock.module('electron', () => ({
    app: { getPath: () => directory },
    ipcMain: { handle: (channel, handler) => handlers.set(channel, handler) },
    shell: { showItemInFolder: (path) => revealed.push(path) },
  }))
  const { installFileActions } = await import('./fileActions')
  installFileActions({
    authorize: (event) => {
      if (!event.trusted) throw new Error('Untrusted renderer')
      return { document: 1 }
    },
    current: () => active,
  })
  const invoke = (action, path = target, event = { trusted: true }) => handlers.get('jaz:file-actions')(event, action, 'thread', path)
  beforeEach(() => {
    active = { phase: 'connected', revision: 1, url: backend.url.origin, session: { token: 'device-token' } }
    probe = 'local'
    replaceConnection = false
    revealed.length = 0
    requests.length = 0
  })
  afterAll(() => {
    backend.stop(true)
    other.stop(true)
    mock.restore()
    rmSync(directory, { recursive: true, force: true })
  })

  test('proves local filesystem access and reveals the server-resolved file', async () => {
    expect(await invoke('canReveal')).toBe(true)
    await invoke('reveal', 'relative.md')
    expect(revealed).toEqual([target])
    expect(new URL(requests.at(-1).url).searchParams.get('path')).toBe('relative.md')
    expect(readdirSync(directory)).toEqual(['report.md'])
  })

  test('a loopback tunnel cannot reveal a coincidentally matching local path', async () => {
    probe = 'remote'
    expect(await invoke('canReveal')).toBe(false)
    await expect(invoke('reveal')).rejects.toThrow('local server')
    expect(revealed).toEqual([])
    expect(readdirSync(directory)).toEqual(['report.md'])
  })

  test('fails closed after connection replacement or a third-party redirect', async () => {
    replaceConnection = true
    await expect(invoke('reveal')).rejects.toThrow('connection changed')
    replaceConnection = 'file'
    await expect(invoke('reveal')).rejects.toThrow('connection changed')
    replaceConnection = false
    probe = 'redirect'
    expect(await invoke('canReveal')).toBe(false)
    expect(redirects).toBe(0)
    expect(revealed).toEqual([])
    expect(readdirSync(directory)).toEqual(['report.md'])
  })

  test('remote origins and untrusted callers cannot probe local paths', async () => {
    active = { ...active, url: 'https://remote.example' }
    expect(await invoke('canReveal')).toBe(false)
    await expect(invoke('reveal', target, { trusted: false })).rejects.toThrow('Untrusted renderer')
    expect(requests).toEqual([])
    expect(revealed).toEqual([])
  })
} else {
  test('native file actions', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_FILE_ACTIONS_TEST_CHILD: '1' },
      encoding: 'utf8',
    })
    if (result.status !== 0) throw new Error(result.stdout + result.stderr)
  })
}
