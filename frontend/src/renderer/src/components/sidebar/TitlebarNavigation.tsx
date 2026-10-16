import { Link } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, createLucideIcon } from 'lucide-react'
import { useSyncExternalStore } from 'react'
import type { BrowserNavigationDirection } from '@shared/browserNavigation'

const CONTROL_CLASS = 'grid size-10 shrink-0 cursor-pointer place-items-center rounded-[14px] bg-clip-content p-1.5 text-ink-2 transition-[color,background-color,transform] duration-150 [-webkit-app-region:no-drag] not-disabled:hover:bg-list-hover not-disabled:hover:text-ink not-disabled:active:scale-[0.96] disabled:cursor-default disabled:opacity-30'

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
  hasPanel,
  panelOpen,
  isMobile,
  onToggleSidebar,
  onNavigate,
}: {
  hasPanel: boolean
  panelOpen: boolean
  isMobile: boolean
  onToggleSidebar: () => void
  onNavigate: (direction: BrowserNavigationDirection) => void
}) {
  useSyncExternalStore(subscribe, () => window.navigation.currentEntry)

  return (
    <div role="group" aria-label="Window navigation" className="flex items-center [&_svg]:stroke-[1.75]">
      {hasPanel && (
        <button
          type="button"
          aria-label={panelOpen ? 'Hide sidebar' : 'Show sidebar'}
          aria-expanded={panelOpen}
          title={`${panelOpen ? 'Hide' : 'Show'} sidebar (⌘S)`}
          onClick={onToggleSidebar}
          className={CONTROL_CLASS}
        >
          <SidebarIcon className="size-4 max-sm:size-[18px]" aria-hidden>
            <path
              d="M9 3H7a4 4 0 0 0-4 4v10a4 4 0 0 0 4 4h2Z"
              fill="currentColor"
              fillOpacity={panelOpen ? 1 : 0}
              stroke="none"
              className="transition-[fill-opacity] duration-150 motion-reduce:transition-none"
            />
          </SidebarIcon>
        </button>
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
            <ArrowLeft size={18} aria-hidden />
          </button>
          <button
            type="button"
            aria-label="Go forward"
            title="Go forward (⌘])"
            disabled={!window.navigation.canGoForward}
            onClick={() => onNavigate('forward')}
            className={CONTROL_CLASS}
          >
            <ArrowRight size={18} aria-hidden />
          </button>
        </>
      )}
      {hasPanel && !panelOpen && (
        <Link to="/new" aria-label="New chat" title="New chat (⌘N)" className={CONTROL_CLASS}>
          <NewChatIcon className="size-4 max-sm:size-[18px]" aria-hidden />
        </Link>
      )}
    </div>
  )
}
