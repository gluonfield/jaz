import { useQuery } from '@tanstack/react-query'
import { Link, linkOptions, useRouterState } from '@tanstack/react-router'
import { LayoutDashboard, MessageSquare, Plus, Settings } from 'lucide-react'
import { useRef, useState } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { BotsGlyph } from '@/components/bots/BotAvatar'
import type { SettingsSection } from '@/components/settings/sections'
import { botsQuery } from '@/lib/api/bots'
import { entrypointKey, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

export const RAIL_WIDTH = 48

// The sections beside Chat. Bots keeps the thread panel for its own list;
// the rest take the whole content card.
export const SECTIONS = [
  { to: '/bots', label: 'Bots', Icon: BotsGlyph },
  { to: '/boards', label: 'Boards', Icon: LayoutDashboard },
] as const

export type RailTab = 'chat' | 'settings' | (typeof SECTIONS)[number]['to'] | `/apps/${string}/${string}`

// Settings rides in the URL search over any page, so it outranks the path.
export function railTab(pathname: string, settingsOpen: boolean): RailTab {
  if (settingsOpen) return 'settings'
  const app = /^\/apps\/([^/]+\/[^/]+)/.exec(pathname)?.[1]
  if (app) return `/apps/${app}` as RailTab
  // Loops open from the Bots panel, so they keep it.
  const section = pathname.startsWith('/loops') ? '/bots' : pathname
  return SECTIONS.find(({ to }) => section.startsWith(to))?.to ?? 'chat'
}

// Everything beside Chat takes the whole content card: the built-in sections,
// then each sidebar app a connected MCP server declares.
export function useRailSections() {
  const apps = (useQuery(mcpEntrypointsQuery).data ?? []).filter((entry) => entry.type === 'global')
  return [
    ...SECTIONS.map(({ to, label, Icon }) => ({ path: to, label, icon: <Icon aria-hidden />, link: linkOptions({ to }) })),
    ...apps.map((app) => ({
      path: `/apps/${entrypointKey(app)}`,
      label: app.title,
      icon: <AppIcon app={app} />,
      link: linkOptions({ to: '/apps/$serverId/$tool', params: { serverId: app.server_id, tool: app.tool } }),
    })),
  ]
}

const TAB_CLASS =
  'group relative grid size-9 place-items-center rounded-[10px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] [&_svg]:size-[18px] [&_svg]:stroke-[1.75]'

const tabClass = (active: boolean) =>
  `${TAB_CLASS} ${active ? 'bg-list-active text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'}`

// The rail is icon-only, so each tab names itself beside the icon on hover.
function TabLabel({ children, shortcut }: { children: string; shortcut?: number }) {
  return (
    <span
      aria-hidden
      className="pointer-events-none absolute left-full top-1/2 z-tooltip ml-2 -translate-y-1/2 whitespace-nowrap rounded-lg bg-surface-2 px-2 py-1 text-[12px] font-medium text-ink opacity-0 shadow-raised ring-1 ring-border/70 transition-opacity duration-100 group-hover:opacity-100 group-hover:delay-150 group-focus-visible:opacity-100"
    >
      {children}
      {shortcut && shortcut <= 9 ? <kbd className="ml-2 font-sans text-ink-3">⌘{shortcut}</kbd> : null}
    </span>
  )
}

export function NavRail({
  tab,
  onOpenChat,
  onOpenSettings,
}: {
  tab: RailTab
  onOpenChat: () => void
  onOpenSettings: (section?: SettingsSection) => void
}) {
  const rail = useRef<HTMLElement>(null)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const sections = useRailSections()
  const botsUnread = useQuery({
    ...botsQuery,
    select: (bots) => bots.some((bot) => bot.unread),
  }).data

  // The Chat tab returns to the thread the user left, the way switching apps
  // does; a fresh /new clears it so the tab lands back on the composer.
  const [lastSession, setLastSession] = useState<string>()
  const session = /^\/sessions\/([^/]+)/.exec(pathname)?.[1]
  const current = session ?? (pathname === '/new' ? undefined : lastSession)
  if (current !== lastSession) setLastSession(current)

  useWindowEvent('keydown', (event) => {
    if (!event.metaKey || event.defaultPrevented || event.altKey || event.ctrlKey || event.shiftKey || event.repeat) return
    if (!/^[1-9]$/.test(event.key) || modalDialogOpen(tab === 'settings' ? 1 : 0)) return
    const target = rail.current?.querySelectorAll<HTMLElement>('[data-rail-tab]')[Number(event.key) - 1]
    if (!target) return
    event.preventDefault()
    target.focus({ preventScroll: true })
    target.click()
  }, tab !== 'chat')

  const sectionTab = (section: (typeof sections)[number], index: number) => (
    <Link
      key={section.path}
      {...section.link}
      data-rail-tab=""
      aria-label={section.path === '/bots' && botsUnread ? 'Bots (unread messages)' : section.label}
      className={tabClass(tab === section.path)}
    >
      {section.icon}
      {section.path === '/bots' && botsUnread && (
        <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
      )}
      <TabLabel shortcut={tab !== 'chat' ? index + 2 : undefined}>{section.label}</TabLabel>
    </Link>
  )

  return (
    <nav ref={rail} aria-label="Sections" style={{ width: RAIL_WIDTH }} className="flex shrink-0 flex-col items-center gap-2 pb-2 pt-[5px] max-sm:hidden">
      <Link
        to={lastSession ? '/sessions/$sessionId' : '/new'}
        params={lastSession ? { sessionId: lastSession } : {}}
        data-rail-tab=""
        aria-label="Chat"
        onClick={onOpenChat}
        className={tabClass(tab === 'chat')}
      >
        <MessageSquare aria-hidden />
        <TabLabel shortcut={tab !== 'chat' ? 1 : undefined}>Chat</TabLabel>
      </Link>
      {sections.slice(0, SECTIONS.length).map(sectionTab)}
      {/* Connected apps sit below the built-in sections, with a way to add more. */}
      <span aria-hidden className="h-px w-4 bg-border" />
      {sections.slice(SECTIONS.length).map((section, index) => sectionTab(section, SECTIONS.length + index))}
      <button
        type="button"
        onClick={() => onOpenSettings('mcp')}
        aria-label="Connect an app"
        className={tabClass(false)}
      >
        <Plus aria-hidden />
        <TabLabel>Connect an app</TabLabel>
      </button>
      <button
        type="button"
        data-rail-tab=""
        onClick={() => onOpenSettings()}
        aria-label="Settings"
        className={`${tabClass(tab === 'settings')} mt-auto`}
      >
        <Settings aria-hidden />
        <TabLabel shortcut={tab !== 'chat' ? sections.length + 2 : undefined}>Settings</TabLabel>
      </button>
    </nav>
  )
}
