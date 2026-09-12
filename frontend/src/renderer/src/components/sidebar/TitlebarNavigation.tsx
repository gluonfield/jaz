import { Link } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, createLucideIcon } from 'lucide-react'
import { useSyncExternalStore } from 'react'
import type { BrowserNavigationDirection } from '@shared/browserNavigation'

const CONTROL_CLASS = 'grid size-10 shrink-0 cursor-pointer place-items-center text-ink-2 transition-[color,background-color,transform] duration-150 [-webkit-app-region:no-drag] not-disabled:hover:bg-surface-2 not-disabled:hover:text-ink not-disabled:active:scale-[0.96] disabled:cursor-default disabled:opacity-30'

const SidebarIcon = createLucideIcon('sidebar-rounded', [
  ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '4', key: 'frame' }],
  ['path', { d: 'M9 3v18', key: 'divider' }],
])

const NewChatIcon = createLucideIcon('new-chat-rounded', [
  ['path', { d: 'M12 3H7a4 4 0 0 0-4 4v10a4 4 0 0 0 4 4h10a4 4 0 0 0 4-4v-5', key: 'frame' }],
  ['path', { d: 'm17 3-8.5 8.5L7 17l5.5-1.5L21 7a2.83 2.83 0 0 0-4-4Z', key: 'pen' }],
])

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
  const controlClass = `${CONTROL_CLASS} ${sidebarOpen ? 'rounded-lg' : 'rounded-xl p-1.5 bg-clip-content'}`

  return (
    <div
      role="group"
      aria-label="Window navigation"
      className={`absolute top-1.5 z-drawer flex items-center [&_svg]:stroke-[1.75] ${
        isMacDesktop && !isMobile ? 'left-[80px]' : 'left-2'
      }`}
    >
      <button
        type="button"
        aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
        aria-expanded={sidebarOpen}
        title={`${sidebarOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
        onClick={onToggleSidebar}
        className={controlClass}
      >
        <SidebarIcon className="size-4 max-sm:size-[18px]" aria-hidden />
      </button>
      {!sidebarOpen && (
        <Link
          to="/new"
          aria-label="New chat"
          title="New chat (⌘N)"
          className={controlClass}
        >
          <NewChatIcon className="size-4 max-sm:size-[18px]" aria-hidden />
        </Link>
      )}
      {sidebarOpen && !isMobile && (
        <>
          <button
            type="button"
            aria-label="Go back"
            title="Go back (⌘[)"
            disabled={!window.navigation.canGoBack}
            onClick={() => onNavigate('back')}
            className={controlClass}
          >
            <ArrowLeft size={18} aria-hidden />
          </button>
          <button
            type="button"
            aria-label="Go forward"
            title="Go forward (⌘])"
            disabled={!window.navigation.canGoForward}
            onClick={() => onNavigate('forward')}
            className={controlClass}
          >
            <ArrowRight size={18} aria-hidden />
          </button>
        </>
      )}
    </div>
  )
}
