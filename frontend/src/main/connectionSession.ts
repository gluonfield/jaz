import { fileURLToPath } from 'node:url'
import { ipcMain, type BrowserWindow, type IpcMainEvent, type IpcMainInvokeEvent, type WebFrameMain } from 'electron'
import { trustedWindow } from '@main/permissions'
import {
  CONNECTION_SESSION_CHANNEL,
  LOCAL_SESSION_CHANNEL,
  deviceAccess,
  validBackendOrigin,
  type LocalSession,
  type SessionDocument,
  type SessionMutation,
  type SessionSnapshot,
} from '@shared/connectionSession'
import type { DeviceIdentity, DeviceMetadata } from '@main/deviceIdentity'

const LOCAL_URL = 'http://127.0.0.1:5299'
type WindowDocument = { window: BrowserWindow; document: number; frame?: WebFrameMain; active: boolean }
type Dependencies = {
  rendererPath: string
  startLocal: () => Promise<{ ok: boolean; url?: string; key?: string; error?: string }>
  profile: () => DeviceIdentity & DeviceMetadata
  stopConsumers: () => void
}

export function createConnectionSession(deps: Dependencies) {
  const windows = new Map<number, WindowDocument>()
  let owner: BrowserWindow | null = null
  let nextDocument = 0
  let snapshot: SessionSnapshot = { revision: 0, phase: 'initial', url: '', session: null, error: null, localPending: false }
  let localGeneration = 0
  let localFlight: { generation: number; promise: Promise<LocalSession>; settled: boolean } | null = null

  function expectedDocument(value: string): boolean {
    try {
      const url = new URL(value)
      const dev = process.env.ELECTRON_RENDERER_URL
      return dev ? url.origin === new URL(dev).origin : url.protocol === 'file:' && fileURLToPath(url) === deps.rendererPath
    } catch {
      return false
    }
  }

  function authorize(event: IpcMainEvent | IpcMainInvokeEvent, document?: number): WindowDocument {
    trustedWindow(event)
    const record = windows.get(event.sender.id)
    if (!record || !record.active || event.senderFrame !== event.sender.mainFrame || !expectedDocument(event.senderFrame.url) ||
      (document !== undefined && record.document !== document)) {
      throw new Error('Connection session requires the current app document')
    }
    return record
  }

  function authorizeOwner(event: IpcMainEvent | IpcMainInvokeEvent, document?: number): WindowDocument {
    const record = authorize(event, document)
    if (record.window !== owner) {
      throw new Error('Only the connection owner can change the session')
    }
    return record
  }

  function deliver(record: WindowDocument): void {
    const contents = record.window.webContents
    if (!record.window.isDestroyed() && !contents.isDestroyed() && record.active && record.frame === contents.mainFrame && expectedDocument(record.frame.url)) {
      record.frame.send(CONNECTION_SESSION_CHANNEL, { document: record.document, snapshot })
    }
  }

  function change(next: Omit<SessionSnapshot, 'revision'>, sender?: BrowserWindow): SessionSnapshot {
    if (next.phase !== 'connected' || next.session?.token !== snapshot.session?.token || next.url !== snapshot.url) {
      deps.stopConsumers()
    }
    snapshot = { ...next, revision: snapshot.revision + 1 }
    for (const record of windows.values()) {
      if (record.window !== sender) {
        deliver(record)
      }
    }
    return snapshot
  }

  function clear(error: string | null, sender?: BrowserWindow): SessionSnapshot {
    localGeneration += 1
    return change({ phase: 'blocked', url: snapshot.url, session: null, error, localPending: false }, sender)
  }

  function attach(window: BrowserWindow, connectionOwner = false): void {
    if (connectionOwner) {
      owner = window
    }
    const record: WindowDocument = { window, document: ++nextDocument, active: false }
    windows.set(window.webContents.id, record)
    window.webContents.on('did-start-navigation', (event) => {
      if (!event.isMainFrame || event.isSameDocument) {
        return
      }
      record.document = ++nextDocument
      record.active = false
      record.frame = undefined
      if (window === owner && snapshot.phase === 'pending' && !snapshot.localPending) {
        clear('Connection interrupted. Connect again.')
      }
    })
    window.webContents.on('did-frame-navigate', (_event, _url, _code, _status, mainFrame) => {
      if (mainFrame) {
        record.active = true
      }
    })
    const contentsId = window.webContents.id
    window.on('closed', () => {
      windows.delete(contentsId)
      if (owner === window) {
        owner = null
        if (snapshot.phase === 'pending' && !snapshot.localPending) {
          clear('Connection interrupted. Connect again.')
        }
      }
    })
  }

  function read(event: IpcMainEvent): SessionDocument {
    const record = authorize(event)
    record.frame = event.senderFrame ?? undefined
    return { document: record.document, owner: record.window === owner, snapshot }
  }

  function update(event: IpcMainEvent, request: SessionMutation): SessionSnapshot {
    if (request.operation === 'reject') {
      authorize(event, request.document)
      if (request.revision === snapshot.revision && snapshot.phase === 'connected') {
        clear('Backend device access was rejected')
      }
      return snapshot
    }
    const record = authorizeOwner(event, request.document)
    if (request.operation === 'clear') {
      return clear(request.error, record.window)
    }
    if (request.operation === 'begin') {
      if (!validBackendOrigin(request.url)) {
        throw new Error('Invalid backend origin')
      }
      if (!(snapshot.localPending && request.url === LOCAL_URL)) {
        localGeneration += 1
      }
      return change({ phase: 'pending', url: request.url, session: null, error: null, localPending: snapshot.localPending && request.url === LOCAL_URL }, record.window)
    }
    const session = request.session
    if (request.revision !== snapshot.revision || (snapshot.phase !== 'pending' && snapshot.phase !== 'connected') ||
      session.url !== snapshot.url || !validBackendOrigin(session.url) || typeof session.token !== 'string' || typeof session.remembered !== 'boolean' ||
      (session.token && (!session.token.startsWith('jaz_') || !session.deviceId))) {
      throw new Error('Connection session was replaced')
    }
    if (localFlight?.settled) {
      localFlight = null
    }
    return change({ phase: 'connected', url: session.url, session, error: null, localPending: false }, record.window)
  }

  async function issueLocal(generation: number): Promise<LocalSession> {
    const local = await deps.startLocal()
    if (generation !== localGeneration) {
      throw new Error('Local startup was cancelled')
    }
    if (!local.ok) {
      throw new Error(local.error || 'Could not start the local backend')
    }
    if (local.url !== LOCAL_URL) {
      throw new Error('Local bootstrap returned the wrong backend')
    }
    const health = await fetch(LOCAL_URL + '/health', { redirect: 'error', signal: AbortSignal.timeout(3000) })
    const healthBody = await health.json() as { ok?: boolean; auth_required?: boolean }
    if (generation !== localGeneration) {
      throw new Error('Local startup was cancelled')
    }
    if (!health.ok || healthBody.ok !== true || typeof healthBody.auth_required !== 'boolean') {
      throw new Error('Local backend is not healthy')
    }
    if (!healthBody.auth_required) {
      return { url: LOCAL_URL, token: '', deviceId: '', remembered: false }
    }
    if (!local.key) {
      throw new Error('Local bootstrap credential is unavailable')
    }
    const profile = deps.profile()
    const response = await fetch(LOCAL_URL + '/v1/devices/register', {
      method: 'POST', redirect: 'error',
      headers: { Authorization: `Bearer ${local.key}`, 'Content-Type': 'application/json', 'X-Jaz-Client-Platform': 'desktop' },
      body: JSON.stringify({ ...profile, kind: 'desktop' }), signal: AbortSignal.timeout(5000),
    })
    const body = await response.json() as { token?: string; error?: string }
    if (!response.ok || !body.token?.startsWith('jaz_')) {
      throw new Error(body.error || 'Local device registration failed')
    }
    const check = await fetch(LOCAL_URL + '/v1/auth/check', {
      redirect: 'error', headers: { Authorization: `Bearer ${body.token}`, 'X-Jaz-Client-Platform': 'desktop' }, signal: AbortSignal.timeout(3000),
    })
    if (!check.ok) {
      throw new Error('Local device verification failed')
    }
    const access = deviceAccess(await check.json(), profile.device_id)
    return { url: LOCAL_URL, token: body.token, deviceId: access.deviceId, ticket: access.ticket, remembered: false }
  }

  async function local(event: IpcMainInvokeEvent, document: number, revision: number): Promise<LocalSession> {
    authorizeOwner(event, document)
    if (snapshot.url !== LOCAL_URL || snapshot.revision !== revision || snapshot.phase !== 'pending') {
      throw new Error('Local startup was cancelled')
    }
    const generation = localGeneration
    snapshot = { ...snapshot, localPending: true }
    if (localFlight && localFlight.generation !== generation) {
      const previous = localFlight
      await previous.promise.catch(() => {})
      authorizeOwner(event, document)
      if (snapshot.revision !== revision || generation !== localGeneration) {
        throw new Error('Local startup was cancelled')
      }
      if (localFlight === previous) {
        localFlight = null
      }
    }
    if (!localFlight) {
      const flight = { generation, promise: issueLocal(generation), settled: false }
      localFlight = flight
      void flight.promise.finally(() => {
        flight.settled = true
      }).catch(() => {})
    }
    try {
      const result = await localFlight.promise
      authorizeOwner(event, document)
      if (snapshot.revision !== revision || generation !== localGeneration) {
        throw new Error('Local startup was cancelled')
      }
      return result
    } catch (error) {
      if (owner && windows.get(owner.webContents.id)?.document === document && snapshot.revision === revision && generation === localGeneration) {
        clear(error instanceof Error ? error.message : 'Local startup failed')
      }
      throw error
    }
  }

  ipcMain.on(CONNECTION_SESSION_CHANNEL, (event, request: SessionMutation | { operation: 'read' }) => {
    try {
      event.returnValue = { value: request.operation === 'read' ? read(event) : update(event, request) }
    } catch (error) {
      event.returnValue = { error: (error as Error).message }
    }
  })
  ipcMain.handle(LOCAL_SESSION_CHANNEL, local)
  return { attach, authorizeOwner, authorize, current: () => snapshot, stop: () => clear(null) }
}
