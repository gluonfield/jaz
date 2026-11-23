import { randomUUID } from 'node:crypto'
import { mkdtemp, rm, stat, writeFile } from 'node:fs/promises'
import { isAbsolute, join } from 'node:path'
import { app, ipcMain, shell } from 'electron'
import type { createConnectionSession } from '@main/connectionSession'
import { trustedWindow } from '@main/permissions'
import { DROPPED_DIRECTORY_CHANNEL, FILE_ACTIONS_CHANNEL } from '@shared/fileActions'
import { isPreviewProxyTargetHostname } from '@shared/preview'
import type { SessionSnapshot } from '@shared/connectionSession'

export function installDroppedDirectory(): void {
  ipcMain.handle(DROPPED_DIRECTORY_CHANNEL, async (event, path: unknown) => {
    trustedWindow(event)
    if (typeof path !== 'string' || !isAbsolute(path)) return null
    const info = await stat(path).catch(() => null)
    return info?.isDirectory() ? path : null
  })
}

export function installFileActions(connection: Pick<ReturnType<typeof createConnectionSession>, 'authorize' | 'current'>): void {
  installDroppedDirectory()
  const read = (active: SessionSnapshot, sessionId: string, path: string, raw = false) => {
    const url = new URL(`/v1/sessions/${encodeURIComponent(sessionId)}/file`, active.url)
    url.search = new URLSearchParams({ path, ...(raw && { raw: '1' }) }).toString()
    return fetch(url, {
      redirect: 'error',
      headers: { Authorization: `Bearer ${active.session?.token ?? ''}`, 'User-Agent': 'Mozilla/5.0' },
      signal: AbortSignal.timeout(5000),
    })
  }
  const canReveal = async (active: SessionSnapshot, sessionId: string) => {
    if (active.phase !== 'connected' || !isPreviewProxyTargetHostname(new URL(active.url).hostname)) return false
    const directory = await mkdtemp(join(app.getPath('temp'), 'jaz-file-access-'))
    try {
      const path = join(directory, 'probe')
      const nonce = randomUUID()
      await writeFile(path, nonce, { mode: 0o600 })
      const response = await read(active, sessionId, path, true)
      return response.ok && await response.text() === nonce
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  }
  ipcMain.handle(FILE_ACTIONS_CHANNEL, async (event, action: 'canReveal' | 'reveal', backendUrl: string, sessionId: string, path?: string) => {
    const document = connection.authorize(event).document
    const active = connection.current()
    if (active.url !== backendUrl) throw new Error('Backend connection changed')
    const available = await canReveal(active, sessionId).catch(() => false)
    connection.authorize(event, document)
    if (connection.current().revision !== active.revision) throw new Error('Backend connection changed')
    if (action === 'canReveal') return available
    if (action !== 'reveal' || !available || !path) throw new Error('Open folder requires Jaz desktop connected to its local server')
    const response = await read(active, sessionId, path)
    if (!response.ok) throw new Error('The file is no longer available')
    const file = await response.json() as { path: string }
    if (!isAbsolute(file.path) || !(await stat(file.path)).isFile()) throw new Error('The file is not on this machine')
    connection.authorize(event, document)
    if (connection.current().revision !== active.revision) throw new Error('Backend connection changed')
    shell.showItemInFolder(file.path)
  })
}
