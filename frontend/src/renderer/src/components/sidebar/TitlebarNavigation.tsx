import { Link } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, PanelLeft, SquarePen } from 'lucide-react'
import { useSyncExternalStore } from 'react'
import type { BrowserNavigationDirection } from '@shared/browserNavigation'

const CONTROL_CLASS = 'grid size-10 shrink-0 cursor-pointer place-items-center rounded-lg text-ink-2 transition-[color,background-color,transform] duration-150 [-webkit-app-region:no-drag] not-disabled:hover:bg-surface-2 not-disabled:hover:text-ink not-disabled:active:scale-[0.96] disabled:cursor-default disabled:opacity-30'

function subscribe(onChange: () => void) {
  window.navigation.addEventListener('currententrychange', onChange)
  return () => window.navigation.removeEventListener('currententrychange', onChange)
}

export function TitlebarNavigation({
  sidebarOpen,
  isMobile,
  isMacDesktop,
  onToggleSidebar,
  onNavigate,
}: {
  sidebarOpen: boolean
  isMobile: boolean
  isMacDesktop: boolean
  onToggleSidebar: () => void
  onNavigate: (direction: BrowserNavigationDirection) => void
}) {
  useSyncExternalStore(subscribe, () => window.navigation.currentEntry)

  return (
    <div
      role="group"
      aria-label="Window navigation"
      className={`absolute top-1.5 z-drawer flex items-center [&_svg]:size-[18px] [&_svg]:stroke-[1.75] max-sm:[&_svg]:size-5 ${
        isMacDesktop && !isMobile ? 'left-[80px]' : 'left-2'
      }`}
    >
      <button
        type="button"
        aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
        aria-expanded={sidebarOpen}
        title={`${sidebarOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
        onClick={onToggleSidebar}
        className={CONTROL_CLASS}
      >
        <PanelLeft aria-hidden />
      </button>
      {!sidebarOpen && (
        <Link
          to="/new"
          aria-label="New chat"
          title="New chat (⌘N)"
          className={CONTROL_CLASS}
        >
          <SquarePen aria-hidden />
        </Link>
      )}
      {!isMobile && (
        <>
          <button
            type="button"
            aria-label="Go back"
            title="Go back (⌘[)"
            disabled={!window.navigation.canGoBack}
            onClick={() => onNavigate('back')}
            className={CONTROL_CLASS}
          >
            <ArrowLeft aria-hidden />
          </button>
          <button
            type="button"
            aria-label="Go forward"
            title="Go forward (⌘])"
            disabled={!window.navigation.canGoForward}
            onClick={() => onNavigate('forward')}
            className={CONTROL_CLASS}
          >
            <ArrowRight aria-hidden />
          </button>
        </>
      )}
    </div>
  )
}
