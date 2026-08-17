import { useQuery } from '@tanstack/react-query'
import { Link, type LinkComponentProps } from '@tanstack/react-router'
import { Inbox, LayoutDashboard, Repeat, Search, Settings, SquarePen } from 'lucide-react'
import { type PointerEvent as ReactPointerEvent, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { ConnectionFooterButton } from '@/components/connection/ConnectionFooterButton'
import { UpdatePanel } from '@/components/update/UpdatePanel'
import { feedQuery } from '@/lib/api/feed'
import { SidebarSessions } from './SidebarSessions'

const NAV_LINK_CLASS =
  'group flex h-[30px] items-center gap-2 rounded-full px-2.5 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover max-sm:h-11 max-sm:px-3 max-sm:text-[15px]'

function NavLink({
  to,
  icon,
  label,
  badge,
}: {
  to: LinkComponentProps['to']
  icon: ReactNode
  label: string
  badge?: ReactNode
}) {
  return (
    <Link to={to} className={NAV_LINK_CLASS} activeProps={{ className: 'bg-list-active!' }}>
      <span className="grid size-[18px] shrink-0 place-items-center">{icon}</span>
      <span className="flex-1">{label}</span>
      {badge}
    </Link>
  )
}

function FeedLink() {
  const feed = useQuery(feedQuery)
  const count = feed.data?.length ?? 0
  return (
    <NavLink
      to="/feed"
      icon={<Inbox size={15} className="text-ink-2 max-sm:size-[18px]" />}
      label="Feed"
      badge={
        count > 0 ? (
          <div className="inline-flex h-[14px] min-w-[14px] items-center justify-center rounded-full bg-ink px-1 text-[9px] font-semibold leading-none tabular-nums text-bg">
            {count > 99 ? '99+' : count}
          </div>
        ) : null
      }
    />
  )
}

export function Sidebar({
  open,
  width,
  mobile = false,
  onDismiss,
  resizing,
  onResizeStart,
  onResizeReset,
  onOpenCommandPalette,
  onOpenSettings,
  onOpenConnect,
}: {
  open: boolean
  width: number
  mobile?: boolean
  onDismiss?: () => void
  resizing?: boolean
  onResizeStart: (e: ReactPointerEvent) => void
  onResizeReset: () => void
  onOpenCommandPalette: () => void
  onOpenSettings: () => void
  onOpenConnect: () => void
}) {
  const navRef = useRef<HTMLElement | null>(null)
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
    <aside
      onClick={
        mobile && onDismiss
          ? (event) => {
              if (!(event.target as HTMLElement).closest('button, input, textarea')) onDismiss()
            }
          : undefined
      }
      className="sidebar-material relative flex h-full shrink-0 flex-col border-r border-border max-sm:w-full!"
      style={{ width }}
    >
      <div className={`h-[52px] shrink-0 ${mobile ? '' : 'titlebar-drag'}`} />

      <div className="flex shrink-0 flex-col pl-1.5 pr-3 pb-px max-sm:px-4">
        <div className="flex items-center gap-px">
          <Link
            to="/new"
            className={`${NAV_LINK_CLASS} min-w-0 flex-1`}
            activeProps={{ className: 'bg-list-active!' }}
          >
            <span className="grid size-[18px] shrink-0 place-items-center">
              <SquarePen size={15} className="text-ink-2 max-sm:size-[18px]" />
            </span>
            <span className="flex-1">New task</span>
          </Link>
          <button
            type="button"
            onClick={onOpenCommandPalette}
            aria-label="Open search"
            className="grid size-[30px] shrink-0 place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-list-hover hover:text-ink focus-visible:bg-list-hover focus-visible:ring-2 focus-visible:ring-primary/40 max-sm:size-11"
          >
            <Search size={15} className="max-sm:size-[18px]" />
          </button>
        </div>
      </div>

      <div
        aria-hidden
        className={`pointer-events-none relative z-[1] h-0 shrink-0 transition-opacity duration-150 ${
          showNavEdge ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <div className="h-px bg-border/70" />
        <div className="absolute inset-x-0 top-px h-5 bg-gradient-to-b from-[var(--sidebar-material-bg)] to-transparent" />
      </div>

      <nav
        ref={navRef}
        onScroll={updateNavEdge}
        className="scrollbar-quiet flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto pl-1.5 pr-3 max-sm:gap-6 max-sm:px-4"
      >
        <div className="flex flex-col gap-px">
          <FeedLink />
          <NavLink
            to="/loops"
            icon={<Repeat size={15} className="text-ink-2 max-sm:size-[18px]" />}
            label="Loops"
          />
          <NavLink
            to="/boards"
            icon={<LayoutDashboard size={15} className="text-ink-2 max-sm:size-[18px]" />}
            label="Boards"
          />
        </div>

        <SidebarSessions open={open} />
      </nav>

      <div className="flex shrink-0 flex-col gap-0.5 border-t border-border pl-1.5 pr-3 py-1.5 max-sm:pl-3">
        <UpdatePanel />
        <ConnectionFooterButton onOpenConnect={onOpenConnect} />
        <button
          type="button"
          onClick={onOpenSettings}
          className="group flex w-full items-center gap-2 rounded-full px-2.5 py-1 text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover max-sm:px-3 max-sm:py-2 max-sm:text-[15px]"
        >
          <span className="grid size-[18px] shrink-0 place-items-center">
            <Settings size={15} className="text-ink-2 max-sm:size-[18px]" />
          </span>
          <span className="flex-1 text-left">Settings</span>
        </button>
      </div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
        onPointerDown={onResizeStart}
        onDoubleClick={onResizeReset}
        className="group absolute inset-y-0 right-0 z-10 flex w-2 cursor-col-resize touch-none justify-end max-sm:hidden"
      >
        <span
          className={`h-full w-px transition-colors duration-150 group-hover:bg-primary/40 ${
            resizing ? 'bg-primary/60' : 'bg-transparent'
          }`}
        />
      </div>
    </aside>
  )
}
