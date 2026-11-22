import { useSyncExternalStore } from 'react'

const KEY = 'jaz.responseLinks.external'
const CHANGED = 'jaz:response-links'

export function responseLinksExternal(): boolean {
  try {
    return window.localStorage.getItem(KEY) === 'true'
  } catch {
    return false
  }
}

function setExternal(external: boolean): void {
  try {
    window.localStorage.setItem(KEY, String(external))
  } catch {
    return
  }
  window.dispatchEvent(new Event(CHANGED))
}

function subscribe(listener: () => void): () => void {
  window.addEventListener(CHANGED, listener)
  window.addEventListener('storage', listener)
  return () => {
    window.removeEventListener(CHANGED, listener)
    window.removeEventListener('storage', listener)
  }
}

export function useResponseLinksExternal(): [boolean, (external: boolean) => void] {
  return [useSyncExternalStore(subscribe, responseLinksExternal, () => false), setExternal]
}
