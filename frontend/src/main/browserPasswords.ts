import { randomUUID } from 'node:crypto'
import { join } from 'node:path'
import { app, ipcMain, session, webContents, type IpcMainInvokeEvent, type WebContents } from 'electron'
import {
  PASSWORD_CHANNEL, PASSWORD_CAPTURE_CHANNEL, PASSWORD_FILL_CHANNEL, PASSWORD_RESULT_CHANNEL, PASSWORD_CHANGED_CHANNEL,
  type BrowserPasswordAction, type BrowserPasswordState,
} from '@shared/browserPasswords'
import { PREVIEW_PARTITION } from '@shared/preview'
import { BrowserPasswordStore, type BrowserPassword } from '@main/browserPasswordStore'
import { isTrustedRendererURL } from '@main/permissions'

type PendingPassword = BrowserPassword & { id: string }

export function installBrowserPasswords(): void {
  const store = new BrowserPasswordStore(join(app.getPath('userData'), 'browser-passwords.enc'))
  const pending = new WeakMap<WebContents, PendingPassword>()
  const errors = new WeakMap<WebContents, string>()
  const watching = new Set<WebContents>()
  const changed = (target: WebContents) => {
    const host = target.hostWebContents
    if (host && !host.isDestroyed()) {
      host.send(PASSWORD_CHANGED_CHANNEL, target.id)
    }
  }

  const watch = (target: WebContents) => {
    if (watching.has(target)) {
      return
    }
    watching.add(target)
    target.once('destroyed', () => watching.delete(target))
    target.on('did-navigate', (_event, url) => {
      if (pending.get(target)?.origin !== passwordOrigin(url)) {
        pending.delete(target)
      }
      errors.delete(target)
      changed(target)
    })
  }

  const state = (target: WebContents): BrowserPasswordState => {
    const origin = passwordOrigin(target.getURL())
    if (!origin) {
      return { origin: '', usernames: [], error: 'Password saving is available on HTTPS pages.' }
    }
    const candidate = pending.get(target)
    const result: BrowserPasswordState = {
      origin,
      usernames: [],
      ...(candidate?.origin === origin ? {
        pending: { id: candidate.id, username: candidate.username, update: false },
      } : {}),
      ...(errors.has(target) ? { error: errors.get(target) } : {}),
    }
    try {
      const records = store.read().filter((entry) => entry.origin === origin)
      result.usernames = records.map((entry) => entry.username)
      if (result.pending) {
        result.pending.update = records.some((entry) => entry.username === candidate?.username)
      }
    } catch (error) {
      result.error = (error as Error).message
    }
    return result
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
        const origin = passwordOrigin(target.getURL())
        if (!origin) {
          throw new Error('Password saving is available on HTTPS pages.')
        }
        errors.delete(target)
        switch (action.kind) {
          case 'capture':
            target.mainFrame.send(PASSWORD_CAPTURE_CHANNEL)
            break
          case 'save':
          case 'dismiss': {
            const candidate = pending.get(target)
            if (!candidate || candidate.id !== action.id || candidate.origin !== origin) {
              throw new Error('This password prompt has expired.')
            }
            if (action.kind === 'save') {
              store.save({ origin, username: candidate.username, password: candidate.password })
            }
            pending.delete(target)
            break
          }
          case 'fill':
          case 'remove': {
            if (action.origin !== origin || typeof action.username !== 'string') {
              throw new Error('The browser has moved to another site.')
            }
            if (action.kind === 'remove') {
              store.remove(origin, action.username)
            } else {
              const credential = store.read().find((entry) => entry.origin === origin && entry.username === action.username)
              if (!credential) {
                throw new Error('This saved password is no longer available.')
              }
              target.mainFrame.send(PASSWORD_FILL_CHANNEL, credential)
            }
            break
          }
          default:
            throw new Error('Unknown password action.')
        }
        if (action.kind === 'save' || action.kind === 'remove') {
          for (const browser of watching) {
            if (passwordOrigin(browser.getURL()) === origin) {
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

  ipcMain.on(PASSWORD_CAPTURE_CHANNEL, (event, value: { origin: string; username: string; password: string }) => {
    const target = event.sender
    if (!isPreview(target) || event.senderFrame !== target.mainFrame) {
      return
    }
    const origin = passwordOrigin(event.senderFrame.url)
    if (!origin || origin !== passwordOrigin(target.getURL()) || value?.origin !== origin || typeof value.username !== 'string' ||
      typeof value.password !== 'string' || !value.password || value.username.length > 1024 || value.password.length > 4096) {
      return
    }
    watch(target)
    const username = value.username.trim()
    pending.set(target, { id: randomUUID(), origin, username, password: value.password })
    try {
      const existing = store.read().find((entry) => entry.origin === origin && entry.username === username)
      if (existing?.password === value.password) {
        pending.delete(target)
      }
      errors.delete(target)
    } catch (error) {
      errors.set(target, (error as Error).message)
    }
    changed(target)
  })

  ipcMain.on(PASSWORD_RESULT_CHANNEL, (event, found: boolean) => {
    const target = event.sender
    if (!isPreview(target) || event.senderFrame !== target.mainFrame || !passwordOrigin(target.getURL())) {
      return
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

function passwordOrigin(value: string): string {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' ? url.origin : ''
  } catch {
    return ''
  }
}
