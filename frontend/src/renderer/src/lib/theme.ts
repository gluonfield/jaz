import { useSyncExternalStore } from 'react'
import { clientRuntime } from './clientRuntime'

// 'system' follows the OS appearance; 'light'/'dark' pin it. Persisted across
// launches and mirrored into the inline anti-FOUC script in index.html.
export type ThemePref = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const KEY = 'jaz.theme'
// Shared with lib/appearance.ts: the cached backend `ui` defaults. The theme
// default lives here too so the pre-paint script and this store agree.
const SERVER_DEFAULTS_KEY = 'jaz.serverDefaults'
const LIGHT_BG = 'oklch(0.963 0.007 262)'
const DARK_BG = 'oklch(0.208 0.007 262)'

const media = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

function isThemePref(v: unknown): v is ThemePref {
  return v === 'light' || v === 'dark' || v === 'system'
}

// Deployment default theme from the connected backend; only used when the user
// hasn't pinned one themselves.
function serverDefaultTheme(): ThemePref | null {
  try {
    const raw = localStorage.getItem(SERVER_DEFAULTS_KEY)
    const theme = raw ? (JSON.parse(raw) as { theme?: unknown }).theme : null
    return isThemePref(theme) ? theme : null
  } catch {
    return null
  }
}

function readStored(): ThemePref {
  const v = localStorage.getItem(KEY)
  if (isThemePref(v)) return v
  return serverDefaultTheme() ?? 'system'
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
  root.style.background = root.classList.contains('vibrant')
    ? 'transparent'
    : resolved === 'dark'
      ? DARK_BG
      : LIGHT_BG
  // keep the native window chrome (macOS traffic lights, scrollbars) in step
  clientRuntime.setNativeTheme?.(p)
}

function notify() {
  for (const l of listeners) l()
}

export function getThemePref(): ThemePref {
  return pref
}

export function setThemePref(next: ThemePref) {
  pref = next
  localStorage.setItem(KEY, next)
  apply(next)
  notify()
}

// Called by lib/appearance.ts after it caches new backend `ui` defaults. Only
// moves clients that haven't pinned a theme; an explicit pick always wins.
export function setServerDefaultTheme(theme?: string) {
  if (localStorage.getItem(KEY)) return
  const next = isThemePref(theme) ? theme : 'system'
  if (next === pref) return
  pref = next
  apply(next)
  notify()
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
  notify()
})

// Keep sibling Electron windows, especially detached board windows, in step
// when the theme switcher writes the shared preference from the main window.
window.addEventListener('storage', (event) => {
  if (event.storageArea !== localStorage || event.key !== KEY) return
  const next = readStored()
  if (next === pref) return
  pref = next
  apply(next)
  notify()
})

// Run once at import so nativeTheme is synced even though the inline FOUC
// script already set the class for first paint.
apply(pref)

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, getThemePref)
  return { theme, resolved: resolveTheme(theme), setTheme: setThemePref }
}
