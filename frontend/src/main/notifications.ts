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

// The first notification makes macOS ask whether Jaz may notify; resolves false
// when the system refuses it.
export function sendTestNotification(): Promise<boolean> {
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
  return shell.openExternal(`x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=${id}`)
}
