import { BrowserWindow, Notification } from 'electron'
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
