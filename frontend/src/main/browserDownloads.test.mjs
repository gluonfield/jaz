import { afterAll, beforeEach, expect, mock, test } from 'bun:test'
import { EventEmitter } from 'node:events'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import process from 'node:process'

if (process.env.JAZ_DOWNLOAD_TEST_CHILD === '1') {
  const directory = mkdtempSync(join(tmpdir(), 'jaz-download-test-'))
  const handlers = new Map()
  const browser = new EventEmitter()
  const sent = []
  const opened = []
  const revealed = []
  const sender = { getType: () => 'window', getURL: () => 'file:///app/out/renderer/index.html', send: (...args) => sent.push(args) }
  sender.mainFrame = { url: sender.getURL() }
  const event = { sender, senderFrame: sender.mainFrame }
  const history = join(directory, 'browser-downloads.json')
  let openError = ''
  mock.module('electron', () => ({
    app: { getPath: () => directory },
    session: { fromPartition: () => browser },
    webContents: { getAllWebContents: () => [sender] },
    ipcMain: { handle: (channel, handler) => handlers.set(channel, handler) },
    shell: {
      openPath: async (path) => {
        opened.push(path)
        return openError
      },
      showItemInFolder: (path) => revealed.push(path),
    },
  }))
  const { setTrustedRendererPath } = await import('./permissions')
  setTrustedRendererPath('/app/out/renderer/index.html')
  const { installBrowserDownloads } = await import('./browserDownloads')
  const invoke = (action, request = event) => handlers.get('jaz:browser-downloads')(request, action)
  const reload = () => {
    browser.removeAllListeners()
    installBrowserDownloads()
  }
  function download(name = 'report.pdf') {
    const item = Object.assign(new EventEmitter(), {
      path: '', received: 0,
      getFilename: () => name,
      getSavePath: () => item.path,
      getStartTime: () => 1700000000,
      getReceivedBytes: () => item.received,
      getTotalBytes: () => 100,
      cancel: () => item.emit('done', {}, 'cancelled'),
    })
    browser.emit('will-download', {}, item)
    return item
  }
  beforeEach(() => {
    rmSync(history, { force: true })
    rmSync(history + '.tmp', { recursive: true, force: true })
    openError = ''
    opened.length = 0
    revealed.length = 0
    sent.length = 0
    reload()
  })
  afterAll(() => {
    mock.restore()
    rmSync(directory, { recursive: true, force: true })
  })

  test('tracks live progress and the actual renamed Save dialog destination', async () => {
    const item = download()
    expect((await invoke()).downloads[0]).toMatchObject({ name: 'report.pdf', state: 'progressing', path: '', receivedBytes: 0 })
    item.path = join(directory, 'renamed.pdf')
    item.received = 30
    item.emit('updated', {}, 'progressing')
    expect(sent.at(-1)[1].downloads[0]).toMatchObject({ name: 'renamed.pdf', receivedBytes: 30, path: item.path })
    item.received = 100
    item.emit('done', {}, 'completed')
    expect(JSON.parse(readFileSync(history, 'utf8'))[0]).toMatchObject({ state: 'completed', receivedBytes: 100, path: item.path })
    reload()
    expect((await invoke()).downloads[0]).toMatchObject({ state: 'completed', name: 'renamed.pdf' })
  })

  test('opens and reveals only recorded completed files; reports removed files and OS failures', async () => {
    const item = download()
    const id = (await invoke()).downloads[0].id
    expect((await invoke({ kind: 'open', id })).error).toContain('no longer available')
    item.path = join(directory, 'report.pdf')
    writeFileSync(item.path, 'fixture')
    item.emit('done', {}, 'completed')
    await invoke({ kind: 'open', id, path: '/arbitrary' })
    await invoke({ kind: 'reveal', id })
    await invoke({ kind: 'folder' })
    expect(opened).toEqual([item.path, directory])
    expect(revealed).toEqual([item.path])
    openError = 'No application can open this file.'
    expect((await invoke({ kind: 'open', id })).error).toBe(openError)
    expect((await invoke({ kind: 'folder' })).error).toBe(openError)
    rmSync(item.path)
    expect((await invoke({ kind: 'reveal', id })).error).toContain('no longer available')
    expect((await invoke({ kind: 'open', id: '../arbitrary' })).error).toContain('no longer in the recent list')
  })

  test('cancellation persists and a restart marks unfinished transfers interrupted', async () => {
    download('cancelled.pdf')
    const id = (await invoke()).downloads[0].id
    await invoke({ kind: 'cancel', id })
    expect((await invoke()).downloads[0].state).toBe('cancelled')
    download('unfinished.pdf')
    reload()
    expect((await invoke()).downloads.map((record) => record.state)).toEqual(['failed', 'cancelled'])
  })

  test('an interrupted transfer stays cancellable until its terminal event', async () => {
    const item = download()
    const id = (await invoke()).downloads[0].id
    item.emit('updated', {}, 'interrupted')
    expect((await invoke()).downloads[0].state).toBe('interrupted')
    await invoke({ kind: 'cancel', id })
    expect((await invoke()).downloads[0].state).toBe('cancelled')
    const failed = download('failed.pdf')
    failed.emit('done', {}, 'interrupted')
    expect((await invoke()).downloads[0].state).toBe('failed')
    reload()
    expect((await invoke()).downloads[0].state).toBe('failed')
  })

  test('retains older active transfers while bounding finished history', async () => {
    const first = download('still-active.pdf')
    for (let index = 0; index < 55; index += 1) {
      download(`${index}.pdf`).emit('done', {}, 'completed')
    }
    const state = await invoke()
    expect(state.downloads).toHaveLength(51)
    expect(state.downloads.at(-1).name).toBe('still-active.pdf')
    first.emit('done', {}, 'interrupted')
    expect((await invoke()).downloads).toHaveLength(50)
  })

  test('denies web pages and subframes; malformed history is surfaced', async () => {
    const untrustedFrame = { url: 'https://example.com' }
    for (const request of [
      { sender: { ...sender, getType: () => 'webview' }, senderFrame: sender.mainFrame },
      { sender, senderFrame: { url: 'file:///app/out/renderer/index.html' } },
      { sender: { ...sender, mainFrame: untrustedFrame }, senderFrame: untrustedFrame },
    ]) {
      await expect(invoke(undefined, request)).rejects.toThrow('managed from Jaz')
    }
    writeFileSync(history, '[{"path":"/arbitrary"}]')
    reload()
    expect((await invoke()).error).toBe('Download history could not be loaded.')
    mkdirSync(history + '.tmp')
    download()
    expect((await invoke()).error).toBe('Download history could not be saved.')
    expect((await invoke()).downloads).toHaveLength(1)
  })
} else {
  test('download lifecycle, persistence and IPC access in an isolated Electron fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_DOWNLOAD_TEST_CHILD: '1' }, encoding: 'utf8', timeout: 10000,
    })
    expect({ status: result.status, error: result.error?.message, output: result.status ? result.stderr : '' }).toEqual({ status: 0, error: undefined, output: '' })
  })
}
