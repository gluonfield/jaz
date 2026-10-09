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

let asked = false

// The first notification also asks macOS for permission, and it fails until
// the user answers that prompt, so the first test keeps retrying for a while.
// Resolves false when the system refuses to show it.
export async function sendTestNotification(): Promise<boolean> {
  const attempts = asked ? 1 : 20
  asked = true
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
    const notification = new Notification({
      title: 'Jaz',
      body: 'Notifications are on.',
      ...(process.platform === 'linux' ? { icon: appIcon } : {}),
    })
    notification.once('show', () => resolve(true))
    notification.once('failed', () => resolve(false))
    notification.show()
  })
}

export async function openNotificationSettings(): Promise<void> {
  if (process.platform !== 'darwin') {
    return shell.openExternal('ms-settings:notifications')
  }
  const id = readFileSync(join(process.execPath, '../../Info.plist'), 'utf8').match(/<key>CFBundleIdentifier<\/key>\s*<string>([^<]+)</)?.[1]
  return shell.openExternal(`x-apple.systempreferences:com.apple.preference.notifications?id=${id}`)
}
