import { useSyncExternalStore } from 'react'
import { isAssistantWakeMode, type AssistantWakeMode } from '@shared/assistant'

const STORAGE_KEY = 'jaz.assistant.wakeMode'
const CHANGE_EVENT = 'jaz:assistant-wake-mode'
const DEFAULT_MODE: AssistantWakeMode = 'dictate'

export function getAssistantWakeMode(): AssistantWakeMode {
  if (typeof localStorage === 'undefined') return DEFAULT_MODE
  const stored = localStorage.getItem(STORAGE_KEY)
  return isAssistantWakeMode(stored) ? stored : DEFAULT_MODE
}

export function setAssistantWakeMode(mode: AssistantWakeMode): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(STORAGE_KEY, mode)
  window.dispatchEvent(new Event(CHANGE_EVENT))
}

function subscribe(listener: () => void): () => void {
  const onStorage = (event: StorageEvent) => {
    if (event.key === STORAGE_KEY) listener()
  }
  window.addEventListener(CHANGE_EVENT, listener)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(CHANGE_EVENT, listener)
    window.removeEventListener('storage', onStorage)
  }
}

export function useAssistantWakeMode(): [AssistantWakeMode, (mode: AssistantWakeMode) => void] {
  const mode = useSyncExternalStore(subscribe, getAssistantWakeMode, () => DEFAULT_MODE)
  return [mode, setAssistantWakeMode]
}
