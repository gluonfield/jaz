import { useEffect, useState } from 'react'

// Matches Tailwind's `sm` breakpoint: below it we treat the client as a phone
// and switch the shell to touch-first layout (full-screen sidebar, larger hit
// targets). Kept in JS only where layout math needs the boolean; everything
// purely visual uses the `max-sm:` CSS variant directly.
const MOBILE_QUERY = '(max-width: 639px)'

function readQuery(query: string): boolean {
  return typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia(query).matches
}

// Synchronous read for state initializers that must avoid a first-paint flash
// (e.g. defaulting the sidebar closed on phones before effects run).
export function isMobileViewport(): boolean {
  return readQuery(MOBILE_QUERY)
}

export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() => readQuery(MOBILE_QUERY))
  useEffect(() => {
    if (!window.matchMedia) return
    const mql = window.matchMedia(MOBILE_QUERY)
    const onChange = () => setIsMobile(mql.matches)
    mql.addEventListener('change', onChange)
    onChange()
    return () => mql.removeEventListener('change', onChange)
  }, [])
  return isMobile
}

export function useViewportWidth(): number {
  const [width, setWidth] = useState(() => (typeof window === 'undefined' ? 0 : window.innerWidth))
  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    onResize()
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return width
}
