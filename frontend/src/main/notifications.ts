import { spawn } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { app, BrowserWindow, Notification, shell } from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import {
  alertPath,
  alertSender,
  ISLAND_HOLD_MS,
  parseNotificationConfig,
  type Alert,
  type NotificationMode,
} from '../shared/notifications'
import { createIsland } from './island'
import { createAlertStream } from './notificationStream'
import { logMain } from '@main/diagnostics'

// Routes classified alerts while Jaz is in the background: to the island when
// it may appear, otherwise to Notification Center, never both.
export function createNotifications(open: (path: string) => void) {
  let mode: NotificationMode = 'off'
  let queue = Promise.resolve()
  const delivered = new Map<string, string>()
  const shown = new Set<Notification>()
  const island = createIsland(open)
  const stream = createAlertStream((alert) => {
    const received = Date.now()
    queue = queue.then(() => deliver(alert, received)).catch((error) => logMain('Alert delivery failed', error))
  })

  async function deliver(alert: Alert, received: number): Promise<void> {
    if (mode === 'off' || delivered.get(alert.sessionId) === alert.key || Date.now() - received > ISLAND_HOLD_MS) return
    delivered.set(alert.sessionId, alert.key)
    if (BrowserWindow.getFocusedWindow()) return
    const needed = alert.state === 'needs_you'
    if (mode !== 'center' && (await island.present(alert, mode === 'notch_ears'))) {
      if (needed) spawn('afplay', ['/System/Library/Sounds/Blow.aiff'], { stdio: 'ignore' }).on('error', () => {})
      return
    }
    if (!Notification.isSupported()) return
    const notification = new Notification({
      title: alert.title || alertSender(alert),
      ...(alert.title ? { subtitle: alertSender(alert) } : {}),
      body: alert.body,
      groupId: alert.sessionId,
      silent: !needed,
      ...(needed ? { sound: 'Blow' } : {}),
      ...(process.platform === 'linux' ? { icon: appIcon } : {}),
    })
    shown.add(notification)
    notification.on('click', () => open(alertPath(alert)))
    notification.on('close', () => shown.delete(notification))
    notification.show()
  }

  app.on('browser-window-focus', () => island.clear())

  return {
    configure(value: unknown): boolean {
      const config = parseNotificationConfig(value)
      mode = config?.mode ?? 'off'
      stream.follow(config && config.mode !== 'off' ? config : null)
      return config !== null
    },
    stop(): void {
      mode = 'off'
      stream.follow(null)
      island.clear()
    },
  }
}

// Resolves whether macOS showed a test notification. Until the user answers
// the permission prompt the first notification raises, notifications fail, so
// callers that may have just raised it retry.
export async function sendTestNotification(attempts: number): Promise<boolean> {
  for (let attempt = 1; attempt <= attempts; attempt++) {
    if (await showTestNotification()) {
      return true
    }
    if (attempt < attempts) {
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
  }
  return false
}

function showTestNotification(): Promise<boolean> {
  return new Promise((resolve) => {
    const notification = new Notification({ title: 'Jaz', body: 'Notifications are on.', sound: 'Blow' })
    const finish = (shown: boolean) => {
      clearTimeout(timeout)
      notification.removeAllListeners()
      resolve(shown)
    }
    const timeout = setTimeout(() => finish(false), 2000)
    notification.once('show', () => finish(true))
    notification.once('failed', (_event, error) => {
      logMain('Test notification failed', error)
      finish(false)
    })
    notification.show()
  })
}

export function openNotificationSettings(): Promise<void> {
  const id = readFileSync(join(process.execPath, '../../Info.plist'), 'utf8').match(/<key>CFBundleIdentifier<\/key>\s*<string>([^<]+)</)?.[1]
  return shell.openExternal(`x-apple.systempreferences:com.apple.preference.notifications?id=${id}`)
}
