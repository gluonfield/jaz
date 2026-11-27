import { randomUUID } from 'node:crypto'
import { ipcMain, session, webContents, type IpcMainInvokeEvent, type WebContents, type WebFrameMain } from 'electron'
import {
  PASSWORD_CHANNEL,
  PASSWORD_CAPTURE_CHANNEL,
  PASSWORD_USERNAME_CHANNEL,
  PASSWORD_FILL_CHANNEL,
  PASSWORD_RESULT_CHANNEL,
  PASSWORD_CHANGED_CHANNEL,
  passwordOrigin,
  type BrowserPasswordAction,
  type BrowserPasswordEntry,
  type BrowserPasswordState,
} from '@shared/browserPasswords'
import { PREVIEW_PARTITION } from '@shared/preview'
import { getBrowserPasswordStore, type BrowserPassword, type BrowserPasswordStore } from '@main/browserPasswordStore'
import { isTrustedRendererURL } from '@main/permissions'

const PENDING_GRACE_MS = 60_000
const USERNAME_TTL_MS = 5 * 60_000
const MAX_REMEMBERED_USERNAMES = 16
const LOGOUT_PATH_PATTERN = /(^|\/)(log-?out|sign-?out)(\/|$)/i

type PendingPassword = BrowserPassword & {
  id: string
  expiresAt: number
  timer?: ReturnType<typeof setTimeout>
}

type ActiveFrameRequest = {
  id: string
  frames: Set<WebFrameMain>
  found: boolean
}

type RememberedUsername = {
  username: string
  expiresAt: number
}

export type BrowserPasswordOptions = {
  now?: () => number
  graceMs?: number
}

function isLogoutURL(value: string): boolean {
  try {
    return LOGOUT_PATH_PATTERN.test(new URL(value).pathname)
  } catch {
    return false
  }
}

export function installBrowserPasswords(
  store: BrowserPasswordStore = getBrowserPasswordStore(),
  options: BrowserPasswordOptions = {},
): void {
  const now = options.now ?? Date.now
  const graceMs = options.graceMs ?? PENDING_GRACE_MS
  const pending = new WeakMap<WebContents, PendingPassword>()
  const errors = new WeakMap<WebContents, string>()
  const lastUsernames = new WeakMap<WebContents, Map<string, RememberedUsername>>()
  const activeRequests = new WeakMap<WebContents, ActiveFrameRequest>()
  const watching = new Set<WebContents>()

  const changed = (target: WebContents) => {
    const host = target.hostWebContents
    if (host && !host.isDestroyed()) {
      host.send(PASSWORD_CHANGED_CHANNEL, target.id)
    }
  }

  const clearPending = (target: WebContents) => {
    const current = pending.get(target)
    if (current?.timer) {
      clearTimeout(current.timer)
    }
    pending.delete(target)
  }

  const activePending = (target: WebContents): PendingPassword | undefined => {
    const candidate = pending.get(target)
    if (!candidate) {
      return undefined
    }
    if (candidate.expiresAt <= now()) {
      clearPending(target)
      return undefined
    }
    return candidate
  }

  const setPending = (target: WebContents, record: BrowserPassword) => {
    const existing = activePending(target)
    const id = existing && existing.origin === record.origin && existing.username === record.username && existing.password === record.password
      ? existing.id
      : randomUUID()
    if (existing?.timer) {
      clearTimeout(existing.timer)
    }
    const expiresAt = now() + graceMs
    const timer = setTimeout(() => {
      const current = pending.get(target)
      if (current?.id === id && current.expiresAt <= now()) {
        pending.delete(target)
        if (!target.isDestroyed()) {
          changed(target)
        }
      }
    }, graceMs)
    timer.unref?.()
    pending.set(target, { ...record, id, expiresAt, timer })
  }

  const rememberUsername = (target: WebContents, origin: string, username: string) => {
    const trimmed = username.trim()
    if (!trimmed || trimmed.length > 1024) {
      return
    }
    let byOrigin = lastUsernames.get(target)
    if (!byOrigin) {
      byOrigin = new Map()
      lastUsernames.set(target, byOrigin)
    }
    const currentNow = now()
    for (const [key, entry] of byOrigin) {
      if (entry.expiresAt <= currentNow) {
        byOrigin.delete(key)
      }
    }
    byOrigin.delete(origin)
    while (byOrigin.size >= MAX_REMEMBERED_USERNAMES) {
      const oldest = byOrigin.keys().next().value
      if (oldest === undefined) {
        break
      }
      byOrigin.delete(oldest)
    }
    byOrigin.set(origin, { username: trimmed, expiresAt: currentNow + USERNAME_TTL_MS })
  }

  const rememberedUsername = (target: WebContents, origin: string): string => {
    const byOrigin = lastUsernames.get(target)
    const entry = byOrigin?.get(origin)
    if (!entry) {
      return ''
    }
    if (entry.expiresAt <= now()) {
      byOrigin?.delete(origin)
      return ''
    }
    return entry.username
  }

  const watch = (target: WebContents) => {
    if (watching.has(target)) {
      return
    }
    watching.add(target)
    target.once('destroyed', () => {
      watching.delete(target)
      clearPending(target)
      lastUsernames.delete(target)
      activeRequests.delete(target)
    })
    target.on('did-navigate', (_event, url) => {
      if (typeof url === 'string' && isLogoutURL(url)) {
        clearPending(target)
        const origin = passwordOrigin(url)
        if (origin) {
          lastUsernames.get(target)?.delete(origin)
        }
      } else {
        activePending(target)
      }
      activeRequests.delete(target)
      errors.delete(target)
      changed(target)
    })
  }

  const liveFrames = (target: WebContents): WebFrameMain[] => {
    const main = target.mainFrame
    if (!main) {
      return []
    }
    const subtree = (main as WebFrameMain & { framesInSubtree?: WebFrameMain[] }).framesInSubtree
    return Array.isArray(subtree) && subtree.length > 0 ? subtree : [main]
  }

  const liveOrigins = (target: WebContents): string[] => {
    const seen = new Set<string>()
    const result: string[] = []
    for (const frame of liveFrames(target)) {
      const origin = passwordOrigin(frame.url)
      if (origin && !seen.has(origin)) {
        seen.add(origin)
        result.push(origin)
      }
    }
    const mainOrigin = passwordOrigin(target.getURL())
    if (mainOrigin && !seen.has(mainOrigin)) {
      result.unshift(mainOrigin)
    }
    return result
  }

  const frameBelongsToTarget = (target: WebContents, frame: WebFrameMain | null | undefined): frame is WebFrameMain =>
    Boolean(frame && liveFrames(target).includes(frame))

  const dispatchToFrames = (target: WebContents, frames: WebFrameMain[], channel: string, payload?: Record<string, unknown>) => {
    if (!frames.length) {
      errors.set(target, 'No suitable login form was found on this page.')
      changed(target)
      return
    }
    const requestId = randomUUID()
    activeRequests.set(target, { id: requestId, frames: new Set(frames), found: false })
    for (const frame of frames) {
      frame.send(channel, payload ? { ...payload, requestId } : { requestId })
    }
  }

  const state = (target: WebContents): BrowserPasswordState => {
    store.tryAutoUnlock()
    const origin = passwordOrigin(target.getURL())
    const origins = liveOrigins(target)
    const candidate = activePending(target)
    if (!origin && !origins.length && !candidate) {
      return { origin: '', usernames: [], entries: [], locked: false, error: 'Password saving is available on HTTPS and localhost pages.' }
    }
    const records = store.cached ?? []
    const mainRecords = origin ? records.filter((entry) => entry.origin === origin) : []
    const entries: BrowserPasswordEntry[] = origins.flatMap((frameOrigin) =>
      records.filter((entry) => entry.origin === frameOrigin).map((entry) => ({ origin: entry.origin, username: entry.username })),
    )
    return {
      origin,
      usernames: mainRecords.map((entry) => entry.username),
      entries,
      locked: !store.cached,
      ...(candidate ? {
        pending: {
          id: candidate.id,
          origin: candidate.origin,
          username: candidate.username,
          update: records.some((entry) => entry.origin === candidate.origin && entry.username === candidate.username),
        },
      } : {}),
      ...(errors.has(target) ? { error: errors.get(target) } : {}),
    }
  }

  const owned = (event: IpcMainInvokeEvent, id: number): WebContents => {
    const target = webContents.fromId(id)
    if (event.sender.getType() !== 'window' || event.senderFrame !== event.sender.mainFrame || !isTrustedRendererURL(event.senderFrame.url) ||
      !target || !isPreview(target) || target.hostWebContents !== event.sender) {
      throw new Error('Passwords must be managed from the owning Jaz browser window.')
    }
    watch(target)
    return target
  }

  ipcMain.handle(PASSWORD_CHANNEL, (event, id: number, action?: BrowserPasswordAction) => {
    const target = owned(event, id)
    try {
      if (action) {
        const origins = liveOrigins(target)
        const candidate = activePending(target)
        if (!origins.length && action.kind !== 'save' && action.kind !== 'dismiss') {
          throw new Error('Password saving is available on HTTPS and localhost pages.')
        }
        errors.delete(target)
        let affectedOrigin = ''
        switch (action.kind) {
          case 'unlock':
            store.read({ allowLegacy: true })
            break
          case 'capture': {
            const eligible = liveFrames(target).filter((frame) => Boolean(passwordOrigin(frame.url)))
            dispatchToFrames(target, eligible, PASSWORD_CAPTURE_CHANNEL)
            break
          }
          case 'save':
          case 'dismiss': {
            if (!candidate || candidate.id !== action.id) {
              throw new Error('This password prompt has expired.')
            }
            if (action.kind === 'save') {
              store.save({ origin: candidate.origin, username: candidate.username, password: candidate.password })
              affectedOrigin = candidate.origin
            }
            clearPending(target)
            break
          }
          case 'fill':
          case 'remove': {
            if (!origins.includes(action.origin) || typeof action.username !== 'string') {
              throw new Error('The browser has moved to another site.')
            }
            if (action.kind === 'remove') {
              store.remove(action.origin, action.username)
              affectedOrigin = action.origin
            } else {
              const credential = store.read({ allowLegacy: true }).find((entry) => entry.origin === action.origin && entry.username === action.username)
              if (!credential) {
                throw new Error('This saved password is no longer available.')
              }
              const matchingFrames = liveFrames(target).filter((frame) => passwordOrigin(frame.url) === action.origin)
              dispatchToFrames(target, matchingFrames, PASSWORD_FILL_CHANNEL, credential)
            }
            break
          }
          default:
            throw new Error('Unknown password action.')
        }
        if (affectedOrigin) {
          for (const browser of watching) {
            if (browser === target || liveOrigins(browser).includes(affectedOrigin)) {
              changed(browser)
            }
          }
        }
      }
      return state(target)
    } catch (error) {
      return { ...state(target), error: (error as Error).message }
    }
  })

  ipcMain.on(PASSWORD_USERNAME_CHANNEL, (event, value: { origin: string; username: string }) => {
    const target = event.sender
    if (!isPreview(target) || !frameBelongsToTarget(target, event.senderFrame)) {
      return
    }
    const origin = passwordOrigin(event.senderFrame.url)
    if (!origin || value?.origin !== origin || typeof value.username !== 'string') {
      return
    }
    watch(target)
    rememberUsername(target, origin, value.username)
  })

  ipcMain.on(PASSWORD_CAPTURE_CHANNEL, (event, value: { origin: string; username: string; password: string }) => {
    const target = event.sender
    if (!isPreview(target) || !frameBelongsToTarget(target, event.senderFrame)) {
      return
    }
    const origin = passwordOrigin(event.senderFrame.url)
    if (!origin || value?.origin !== origin || typeof value.username !== 'string' ||
      typeof value.password !== 'string' || !value.password || value.username.length > 1024 || value.password.length > 4096) {
      return
    }
    watch(target)
    store.tryAutoUnlock()
    const username = value.username.trim() || rememberedUsername(target, origin)
    if (username) {
      rememberUsername(target, origin, username)
    }
    const existing = store.cached?.find((entry) => entry.origin === origin && entry.username === username)
    if (existing?.password === value.password) {
      clearPending(target)
    } else {
      setPending(target, { origin, username, password: value.password })
    }
    errors.delete(target)
    changed(target)
  })

  ipcMain.on(PASSWORD_RESULT_CHANNEL, (event, payload: boolean | { requestId?: string; found: boolean }) => {
    const target = event.sender
    if (!isPreview(target) || !frameBelongsToTarget(target, event.senderFrame)) {
      return
    }
    const found = typeof payload === 'boolean' ? payload : Boolean(payload?.found)
    const requestId = typeof payload === 'object' && payload ? payload.requestId : undefined
    if (requestId) {
      const tracked = activeRequests.get(target)
      if (!tracked || tracked.id !== requestId || !tracked.frames.delete(event.senderFrame)) {
        return
      }
      if (found) {
        tracked.found = true
        activeRequests.delete(target)
        errors.delete(target)
        changed(target)
        return
      }
      if (tracked.frames.size > 0) {
        return
      }
      activeRequests.delete(target)
    }
    if (!found) {
      errors.set(target, 'No suitable login form was found on this page.')
    }
    changed(target)
  })
}

function isPreview(target: WebContents): boolean {
  return !target.isDestroyed() && target.getType() === 'webview' && target.session === session.fromPartition(PREVIEW_PARTITION) &&
    Boolean(target.hostWebContents && isTrustedRendererURL(target.hostWebContents.getURL()))
}
