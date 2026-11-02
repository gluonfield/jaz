import { useSyncExternalStore } from 'react'
import { isNotificationMode, type NotificationMode } from '@shared/notifications'

const STORAGE_KEY = 'jaz.notifications.mode'
const LEGACY_KEY = 'jaz.notifications.threadFinished'
const CHANGE_EVENT = 'jaz:notification-mode'

function notificationMode(): NotificationMode {
  if (typeof window === 'undefined') return 'center'
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (isNotificationMode(stored)) return stored
    return window.localStorage.getItem(LEGACY_KEY) === 'false' ? 'off' : 'center'
  } catch {
    return 'center'
  }
}

export function setNotificationMode(value: NotificationMode): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, value)
    window.localStorage.removeItem(LEGACY_KEY)
  } catch {
    return
  }
  window.dispatchEvent(new Event(CHANGE_EVENT))
}

function subscribe(callback: () => void): () => void {
  const listener = () => callback()
  window.addEventListener(CHANGE_EVENT, listener)
  window.addEventListener('storage', listener)
  return () => {
    window.removeEventListener(CHANGE_EVENT, listener)
    window.removeEventListener('storage', listener)
  }
}

export function useNotificationMode(): [NotificationMode, (value: NotificationMode) => void] {
  const value = useSyncExternalStore(subscribe, notificationMode, () => 'center' as const)
  return [value, setNotificationMode]
}
