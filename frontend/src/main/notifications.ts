import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { BrowserWindow, Notification, shell } from 'electron'
import appIcon from '../assets/jaz-icon-1024.png?asset'
import { threadNotificationPath, type ThreadCompletion } from '../shared/notifications'
import { createThreadCompletionMonitor } from './notificationMonitor'

export function createThreadNotificationMonitor(openInMain: (path: string) => void) {
  return createThreadCompletionMonitor((completion: ThreadCompletion) => {
    if (BrowserWindow.getFocusedWindow() || !Notification.isSupported()) return
    const notification = new Notification({
      title: completion.title || completion.slug || 'Untitled thread',
      body: 'Jaz finished this thread.',
      ...(process.platform === 'linux' ? { icon: appIcon } : {}),
    })
    notification.on('click', () => openInMain(threadNotificationPath(completion.id)))
    notification.show()
  })
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
    const notification = new Notification({ title: 'Jaz', body: 'Notifications are on.' })
    notification.once('show', () => resolve(true))
    notification.once('failed', () => resolve(false))
    notification.show()
  })
}

export function openNotificationSettings(): Promise<void> {
  const id = readFileSync(join(process.execPath, '../../Info.plist'), 'utf8').match(/<key>CFBundleIdentifier<\/key>\s*<string>([^<]+)</)?.[1]
  return shell.openExternal(`x-apple.systempreferences:com.apple.preference.notifications?id=${id}`)
}
