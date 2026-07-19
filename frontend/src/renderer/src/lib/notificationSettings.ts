import { useSyncExternalStore } from 'react'

const STORAGE_KEY = 'jaz.notifications.threadFinished'
const CHANGE_EVENT = 'jaz:thread-notifications'

function threadNotificationsEnabled(): boolean {
  if (typeof window === 'undefined') return true
  try {
    return window.localStorage.getItem(STORAGE_KEY) !== 'false'
  } catch {
    return true
  }
}

export function setThreadNotificationsEnabled(value: boolean): void {
  if (typeof window === 'undefined') return
  try {
    if (value) window.localStorage.removeItem(STORAGE_KEY)
    else window.localStorage.setItem(STORAGE_KEY, 'false')
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

export function useThreadNotificationsEnabled(): [boolean, (value: boolean) => void] {
  const value = useSyncExternalStore(subscribe, threadNotificationsEnabled, () => true)
  return [value, setThreadNotificationsEnabled]
}
