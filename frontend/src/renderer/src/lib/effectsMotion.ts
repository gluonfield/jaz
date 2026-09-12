import { useSyncExternalStore } from 'react'
import { useEffectsEnabled } from './appearance'

const query = '(prefers-reduced-motion: reduce)'

function subscribe(notify: () => void) {
  const media = window.matchMedia(query)
  media.addEventListener('change', notify)
  return () => media.removeEventListener('change', notify)
}

function reducedMotion() {
  return window.matchMedia(query).matches
}

export function useReducedEffectsMotion(): boolean {
  const reduced = useSyncExternalStore(subscribe, reducedMotion, () => true)
  const effectsEnabled = useEffectsEnabled()
  return reduced || !effectsEnabled
}
