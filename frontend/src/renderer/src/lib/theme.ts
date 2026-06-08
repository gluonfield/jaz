import { useSyncExternalStore } from 'react'

// 'system' follows the OS appearance; 'light'/'dark' pin it. Persisted across
// launches and mirrored into the inline anti-FOUC script in index.html.
export type ThemePref = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const KEY = 'jaz.theme'

const media = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

function readStored(): ThemePref {
  const v = localStorage.getItem(KEY)
  return v === 'light' || v === 'dark' || v === 'system' ? v : 'system'
}

let pref: ThemePref = readStored()

export function resolveTheme(p: ThemePref): ResolvedTheme {
  if (p === 'system') return media.matches ? 'dark' : 'light'
  return p
}

function apply(p: ThemePref) {
  const resolved = resolveTheme(p)
  const root = document.documentElement
  root.classList.toggle('dark', resolved === 'dark')
  root.style.colorScheme = resolved
  // keep the native window chrome (macOS traffic lights, scrollbars) in step
  window.jaz?.setNativeTheme?.(p)
}

export function getThemePref(): ThemePref {
  return pref
}

export function setThemePref(next: ThemePref) {
  pref = next
  localStorage.setItem(KEY, next)
  apply(next)
  for (const l of listeners) l()
}

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

// A system-appearance flip only changes what we render while pref is 'system'.
media.addEventListener('change', () => {
  if (pref !== 'system') return
  apply('system')
  for (const l of listeners) l()
})

// Run once at import so nativeTheme is synced even though the inline FOUC
// script already set the class for first paint.
apply(pref)

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, getThemePref)
  return { theme, resolved: resolveTheme(theme), setTheme: setThemePref }
}
