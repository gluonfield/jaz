import { Link } from '@tanstack/react-router'
import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { useRailSections } from './NavRail'

export const NAV_LINK_CLASS =
  'group flex h-[30px] items-center gap-2 rounded-lg px-2.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover max-sm:h-11 max-sm:px-3 max-sm:text-[15px]'

export const PANEL_ICON_BUTTON_CLASS =
  'grid size-[30px] shrink-0 place-items-center rounded-lg text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink focus-visible:bg-list-hover focus-visible:ring-2 focus-visible:ring-primary/40 max-sm:size-11'

// The panel's top row: its title or primary action, then icon buttons.
export function SidebarHeader({ children }: { children: ReactNode }) {
  return (
    <div className="flex shrink-0 flex-col pl-1.5 pr-3 pt-1.5 pb-px max-sm:px-4 max-sm:pt-2">
      <div className="flex items-center gap-px">{children}</div>
    </div>
  )
}

// The panel's scrolling body. A hairline and fade appear once content slides
// under the header.
// `tight` starts the content right under a header of icons, with no title to
// set it apart.
export function SidebarScroll({ mobile, tight = false, children }: { mobile: boolean; tight?: boolean; children: ReactNode }) {
  const navRef = useRef<HTMLElement | null>(null)
  const sections = useRailSections()
  const [navEdge, setNavEdge] = useState({ scrollable: false, scrolled: false })
  const updateNavEdge = useCallback(() => {
    const nav = navRef.current
    const scrollable = Boolean(nav && nav.scrollHeight - nav.clientHeight > 1)
    const scrolled = Boolean(scrollable && nav && nav.scrollTop > 1)
    setNavEdge((current) =>
      current.scrollable === scrollable && current.scrolled === scrolled
        ? current
        : { scrollable, scrolled },
    )
  }, [])

  useEffect(() => {
    updateNavEdge()
    const nav = navRef.current
    if (!nav) return

    const resizeObserver = new ResizeObserver(updateNavEdge)
    resizeObserver.observe(nav)
    const mutationObserver = new MutationObserver(updateNavEdge)
    mutationObserver.observe(nav, { childList: true, subtree: true })
    window.addEventListener('resize', updateNavEdge)
    const frame = window.requestAnimationFrame(updateNavEdge)

    return () => {
      window.cancelAnimationFrame(frame)
      window.removeEventListener('resize', updateNavEdge)
      mutationObserver.disconnect()
      resizeObserver.disconnect()
    }
  }, [updateNavEdge])

  const showNavEdge = navEdge.scrollable && navEdge.scrolled

  return (
    <>
      <div
        aria-hidden
        className={`pointer-events-none relative z-[1] h-0 shrink-0 transition-opacity duration-150 ${
          showNavEdge ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <div className="h-px bg-border/70" />
        {/* Only the phone drawer has an opaque panel to fade from; on desktop the list sits on the window. */}
        <div className="absolute inset-x-0 top-px h-5 bg-gradient-to-b from-panel to-transparent sm:hidden" />
      </div>

      <nav
        ref={navRef}
        onScroll={updateNavEdge}
        className={`scrollbar-quiet flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto pl-1.5 pr-3 max-sm:gap-6 max-sm:px-4 ${tight ? 'pt-1' : 'pt-4'}`}
      >
        {/* Phones have no rail, so its tabs ride at the top of the drawer. */}
        {mobile && (
          <div className="flex flex-col gap-px">
            {sections.map((section) => (
              <Link
                key={section.path}
                {...section.link}
                className={NAV_LINK_CLASS}
                activeProps={{ className: 'bg-list-active!' }}
              >
                <span className="grid size-[18px] shrink-0 place-items-center text-ink-2 [&_svg]:size-[18px]">
                  {section.icon}
                </span>
                <span className="flex-1">{section.label}</span>
              </Link>
            ))}
          </div>
        )}

        {children}
      </nav>
    </>
  )
}
