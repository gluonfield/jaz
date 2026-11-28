import { useQuery } from '@tanstack/react-query'
import { Link, linkOptions, useRouterState } from '@tanstack/react-router'
import { AppWindow, MessageSquare, Settings } from 'lucide-react'
import { useRef, useState } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { PluginIcon } from '@/components/plugins/PluginIcon'
import { SidebarWorkspaceSwitcher } from '@/components/sidebar/SidebarWorkspaceSwitcher'
import { BotsGlyph } from '@/components/bots/BotAvatar'
import type { SettingsSection } from '@/components/settings/sections'
import { botsQuery } from '@/lib/api/bots'
import { entrypointKey, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { pluginsQuery } from '@/lib/api/plugins'
import { useCloudSession } from '@/lib/cloudSession'
import { modalDialogOpen } from '@/lib/dom/modal'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

export const RAIL_WIDTH = 48

// The sections beside Chat. Bots keeps the thread panel for its own list;
// the rest take the whole content card.
export const SECTIONS = [
  { to: '/bots', label: 'Bots', Icon: BotsGlyph },
  { to: '/apps', label: 'Apps', Icon: AppWindow },
] as const

export type RailTab = 'chat' | 'settings' | (typeof SECTIONS)[number]['to'] | `/apps/${string}/${string}` | `/plugins/${string}`

// Settings rides in the URL search over any page, so it outranks the path.
export function railTab(pathname: string, settingsOpen: boolean): RailTab {
  if (settingsOpen) return 'settings'
  const mcpApp = /^\/apps\/([^/]+\/[^/]+)/.exec(pathname)?.[1]
  if (mcpApp) return `/apps/${mcpApp}` as RailTab
  const plugin = /^\/plugins\/([^/]+)$/.exec(pathname)?.[1]
  if (plugin) return `/plugins/${plugin}`
  // Loops open from the Bots panel, so they keep it.
  const section = pathname.startsWith('/loops') ? '/bots' : pathname
  return SECTIONS.find(({ to }) => section === to || section.startsWith(`${to}/`))?.to ?? 'chat'
}

// Everything beside Chat takes the whole content card: the built-in sections,
// then each sidebar app a connected MCP server declares.
export function useRailSections() {
  const apps = (useQuery(mcpEntrypointsQuery).data ?? []).filter((entry) => entry.type === 'global')
  const plugins = useQuery(pluginsQuery).data ?? []
  return [
    ...SECTIONS.map(({ to, label, Icon }) => ({ path: to, label, icon: <Icon aria-hidden />, link: linkOptions({ to }) })),
    ...apps.map((app) => ({
      path: `/apps/${entrypointKey(app)}`,
      label: app.title,
      icon: <AppIcon app={app} />,
      link: linkOptions({ to: '/apps/$serverId/$tool', params: { serverId: app.server_id, tool: app.tool } }),
    })),
    ...plugins.flatMap((plugin) => plugin.installed && plugin.enabled && plugin.page ? [{
      path: `/plugins/${plugin.name}`,
      label: plugin.display_name,
      icon: <PluginIcon icon={plugin.icon_data_url ?? ''} />,
      link: linkOptions({ to: '/plugins/$plugin', params: { plugin: plugin.name } }),
    }] : []),
  ]
}

const TAB_CLASS =
  'relative grid size-9 place-items-center rounded-[10px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] [&_svg]:size-[18px] [&_svg]:stroke-[1.75]'

const tabClass = (active: boolean) =>
  `${TAB_CLASS} ${active ? 'bg-list-active text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'}`

// The rail is icon-only, so each tab names itself in a tooltip beside the icon.
const tabTip = (label: string, shortcut?: number) => ({
  title: label,
  'data-tooltip-side': 'right',
  'aria-keyshortcuts': shortcut && shortcut <= 9 ? `Meta+${shortcut}` : undefined,
})

export function NavRail({
  tab,
  onOpenSidebar,
  onOpenSettings,
}: {
  tab: RailTab
  onOpenSidebar: () => void
  onOpenSettings: (section?: SettingsSection) => void
}) {
  const rail = useRef<HTMLElement>(null)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { activeWorkspaceId } = useCloudSession()
  const sections = useRailSections()
  const botsUnread = useQuery({
    ...botsQuery,
    select: (bots) => bots.some((bot) => bot.unread),
  }).data

  // The Chat tab returns to the thread the user left, the way switching apps
  // does; a fresh /new or workspace switch clears it so the tab lands back on
  // the composer instead of a previous workspace's thread.
  const [lastSession, setLastSession] = useState<{ workspaceId: string; id?: string }>({
    workspaceId: activeWorkspaceId,
  })
  const session = /^\/sessions\/([^/]+)/.exec(pathname)?.[1]
  const activeLastSession = lastSession.workspaceId === activeWorkspaceId ? lastSession.id : undefined
  const current = session ?? (pathname === '/new' ? undefined : activeLastSession)
  if (lastSession.workspaceId !== activeWorkspaceId || current !== lastSession.id) {
    setLastSession({ workspaceId: activeWorkspaceId, id: current })
  }

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
      onClick={section.path === '/bots' ? onOpenSidebar : undefined}
      className={tabClass(tab === section.path)}
      {...tabTip(section.label, tab !== 'chat' ? index + 2 : undefined)}
    >
      {section.icon}
      {section.path === '/bots' && botsUnread && (
        <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
      )}
    </Link>
  )

  return (
    <nav ref={rail} aria-label="Sections" style={{ width: RAIL_WIDTH }} className="flex shrink-0 flex-col items-center gap-2 pb-2 pt-[5px] max-sm:hidden">
      <SidebarWorkspaceSwitcher rail />
      <Link
        to={current ? '/sessions/$sessionId' : '/new'}
        params={current ? { sessionId: current } : {}}
        data-rail-tab=""
        aria-label="Chat"
        onClick={onOpenSidebar}
        className={tabClass(tab === 'chat')}
        {...tabTip('Chat', tab !== 'chat' ? 1 : undefined)}
      >
        <MessageSquare aria-hidden />
      </Link>
      {sections.slice(0, SECTIONS.length).map(sectionTab)}
      {sections.length > SECTIONS.length ? (
        <>
          <span aria-hidden className="h-px w-4 bg-border" />
          {sections.slice(SECTIONS.length).map((section, index) => sectionTab(section, SECTIONS.length + index))}
        </>
      ) : null}
      <button
        type="button"
        data-rail-tab=""
        onClick={() => onOpenSettings()}
        aria-label="Settings"
        className={`${tabClass(tab === 'settings')} mt-auto`}
        {...tabTip('Settings', tab !== 'chat' ? sections.length + 2 : undefined)}
      >
        <Settings aria-hidden />
      </button>
    </nav>
  )
}
