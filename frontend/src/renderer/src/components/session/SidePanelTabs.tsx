import { AppWindow, Copy, PencilRuler, FileSpreadsheet, FileText, FileType, FolderGit2, Globe, LayoutDashboard, MessageCirclePlus, Plus, RotateCw, Terminal, X, type LucideIcon } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { motion, Reorder } from 'motion/react'
import { useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { CopyPathMenuRow } from '@/components/session/FileActions'
import { IconButton } from '@/components/ui/IconButton'
import { ContextMenu, MenuRow, Popover } from '@/components/ui/Popover'
import { Favicon } from '@/components/ui/Favicon'
import { isMarkdownPath, isSpreadsheetPath, isWordPath, type FileReference } from '@shared/fileReader'
import { appsQuery } from '@/lib/api/apps'
import { sessionCanvasQuery } from '@/lib/api/canvas'
import type { ChatMessage, Session, SessionEvent } from '@/lib/api/types'
import { publishedAppTab } from '@/lib/sidePanelTabs'
import { entrypointKey, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { useBrowserSessions } from '@/lib/browserSessions'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { useRefetchAfterWork } from '@/lib/hooks/useRefetchAfterWork'
import type { NewSidePanelTab, SidePanelTab } from '@/lib/sidePanelTabs'
import type { LiveExchange } from '@/components/session/liveTranscript'
import { threadPanelItems } from '@/components/session/threadPanelItems'

const MENU_TYPES = {
  file: { label: 'Files', icon: FileText },
  'side-chat': { label: 'Side chat', icon: MessageCirclePlus },
  preview: { label: 'Browser', icon: Globe },
  terminal: { label: 'Terminal', icon: Terminal },
  diff: { label: 'Code diff', icon: FolderGit2 },
}

const TAB_TYPES = { ...MENU_TYPES, canvas: { label: 'Canvas', icon: PencilRuler }, artifact: { label: 'Artifact', icon: AppWindow }, 'published-app': { label: 'App', icon: AppWindow } }
const RECENT_APPS = 5

function KindIcon({ kind, path = '' }: { kind: keyof typeof TAB_TYPES; path?: string }) {
  const spreadsheet = isSpreadsheetPath(path)
  const Icon = spreadsheet ? FileSpreadsheet : isWordPath(path) ? FileType : TAB_TYPES[kind].icon
  return <Icon size={14} className={`shrink-0 ${spreadsheet ? 'text-emerald-600 dark:text-emerald-400' : 'text-ink-3'}`} aria-hidden />
}

function PanelRow({ icon, label, onClick }: { icon: ReactNode; label: string; onClick: () => void }) {
  return <button type="button" className="flex h-7 w-full cursor-pointer items-center gap-2 rounded-lg px-2 text-left text-[13px] text-ink-2 hover:bg-surface-2 hover:text-ink pointer-coarse:h-10" onClick={onClick}>
    <span className="grid size-3.5 shrink-0 place-items-center text-ink-3">{icon}</span>
    <span className="max-w-56 truncate">{label}</span>
  </button>
}

function BuiltInRows({ sideChatAvailable, onAdd }: {
  sideChatAvailable: boolean
  onAdd: (tab: NewSidePanelTab) => void
}) {
  return (Object.keys(MENU_TYPES) as (keyof typeof MENU_TYPES)[])
    .filter((kind) => kind !== 'side-chat' || sideChatAvailable)
    .map((kind) => {
      const { label, icon: Icon } = MENU_TYPES[kind]
      return <PanelRow key={kind} label={label} icon={<Icon size={14} />} onClick={() => onAdd(kind)} />
    })
}

export function SidePanelLauncher({ session, messages, events, live, readCanvas, working, sideChatAvailable, onAdd, onOpenFile }: {
  session: Session
  messages: ChatMessage[]
  events: SessionEvent[]
  live: LiveExchange | null
  readCanvas: boolean
  working: boolean
  sideChatAvailable: boolean
  onAdd: (tab: NewSidePanelTab) => void
  onOpenFile: (file: FileReference) => void
}) {
  const { data: apps } = useQuery(appsQuery({ scope: 'mine', sourceThreadId: session.id }))
  const { data: canvas, refetch } = useQuery({ ...sessionCanvasQuery(session.id), enabled: readCanvas })
  useRefetchAfterWork(working && readCanvas, refetch)
  const items = useMemo(() => threadPanelItems({ session, messages, events, live, apps, canvas }), [session, messages, events, live, apps, canvas])
  return <div className="m-auto max-h-full w-52 overflow-y-auto p-2">
    <BuiltInRows sideChatAvailable={sideChatAvailable} onAdd={onAdd} />
    {items.map(({ id, label, tab }) => <PanelRow key={id} label={label}
      icon={<KindIcon kind={tab.kind} path={tab.kind === 'file' ? tab.file.path : undefined} />}
      onClick={() => tab.kind === 'file' ? onOpenFile(tab.file) : onAdd(tab)}
    />)}
  </div>
}

function SidePanelTabMenu({ sideChatAvailable, onAdd }: {
  sideChatAvailable: boolean
  onAdd: (tab: NewSidePanelTab) => void
}) {
  const [open, setOpen] = useState(false)
  const apps = (useQuery({ ...mcpEntrypointsQuery, enabled: open }).data ?? []).filter((entry) => entry.type === 'thread')
  const published = (useQuery({ ...appsQuery({ scope: 'all' }), enabled: open }).data ?? [])
    .toSorted((a, b) => b.updated_at.localeCompare(a.updated_at))
    .slice(0, RECENT_APPS)
  const add = (tab: NewSidePanelTab) => {
    onAdd(tab)
    setOpen(false)
  }
  return (
    <Popover open={open} onClose={() => setOpen(false)} placement="below" align="end" trigger={
      <IconButton size="sm" className="pointer-coarse:size-10" aria-label="New tab" aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((value) => !value)}>
        <Plus size={15} />
      </IconButton>
    }>
      <BuiltInRows sideChatAvailable={sideChatAvailable} onAdd={add} />
      {apps.map((app) => <PanelRow key={entrypointKey(app)} label={app.title} icon={<AppIcon app={app} size={14} />} onClick={() => add(app)} />)}
      {published.map((app) => <PanelRow key={app.id} label={app.title} icon={<AppWindow size={14} />} onClick={() => add(publishedAppTab(app))} />)}
    </Popover>
  )
}

export function SidePanelTabs({ tabs, activeId, sideChatAvailable, onSelect, onReorder, onClose, onAdd, onDuplicate }: {
  tabs: SidePanelTab[]
  activeId?: string
  sideChatAvailable: boolean
  onSelect: (id: string) => void
  onReorder: (ids: string[]) => void
  onClose: (id: string) => void
  onAdd: (tab: NewSidePanelTab, after?: string) => void
  onDuplicate: (id: string) => void
}) {
  const sessions = useBrowserSessions()
  const browsers = useSyncExternalStore(sessions.subscribe, sessions.getSnapshot)
  const list = useRef<HTMLDivElement>(null)
  const [menu, setMenu] = useState<{ point: { x: number; y: number }; id: string | null } | null>(null)
  const menuTriggers = useContextMenuTrigger((point, target) => setMenu({ point, id: target.getAttribute('data-tab-id') }))
  const menuIndex = tabs.findIndex((tab) => tab.id === menu?.id)
  const menuTab = tabs[menuIndex]
  const menuPath = menuTab?.kind === 'file' || menuTab?.kind === 'app' ? menuTab.file?.path : undefined
  useEffect(() => {
    list.current?.querySelector('[aria-selected="true"]')?.parentElement?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [activeId])
  const focusTab = (id: string) => {
    onSelect(id)
    requestAnimationFrame(() => document.getElementById(`panel-tab-${id}`)?.focus())
  }
  const closeTabs = (closing: SidePanelTab[]) => {
    closing.forEach((tab) => onClose(tab.id))
    requestAnimationFrame(() => {
      const selected = list.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')
      const target = selected ?? list.current?.parentElement?.querySelector<HTMLButtonElement>('[aria-label="New tab"]')
      target?.focus()
    })
  }
  const menuRow = (Icon: LucideIcon, label: string, action: () => void) => (
    <MenuRow role="menuitem" onClick={() => {
      setMenu(null)
      action()
    }}>
      <span className="flex items-center gap-2">
        <Icon size={15} aria-hidden />
        {label}
      </span>
    </MenuRow>
  )
  return (
    <div className="flex h-9 min-w-0 flex-1 items-center gap-1 px-1 pointer-coarse:h-11">
      <Reorder.Group as="div" axis="x" values={tabs.map((tab) => tab.id)} onReorder={onReorder} layoutScroll ref={list} role="tablist" aria-label="Side panel tabs" className="flex min-w-0 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden [-webkit-app-region:no-drag]">
        {tabs.map((tab, index) => {
          const label = tab.kind === 'app' ? tab.file?.name ?? tab.app.title
            : tab.kind === 'view' ? tab.view.title
              : tab.kind === 'artifact' ? tab.artifact.title
                : tab.kind === 'published-app' ? tab.app.title
                  : TAB_TYPES[tab.kind].label
          const target = browsers.find((browser) => browser.id === tab.id)?.target
          const title = tab.kind === 'file' ? tab.file?.path.split(/[\\/]/).pop() || label : target?.title || target?.displayUrl || label
          const active = activeId === tab.id
          return (
            <Reorder.Item as="div" value={tab.id} key={tab.id} data-tab-id={tab.id} {...menuTriggers} layout="position" transition={{ layout: { duration: 0 } }} dragMomentum={false} className={`group relative flex h-7 min-w-0 max-w-full shrink-0 select-none items-center rounded-lg pointer-coarse:h-10 ${active ? 'bg-surface text-ink' : 'text-ink-2 hover:bg-surface-2'}`}>
              <motion.button
                id={`panel-tab-${tab.id}`}
                type="button"
                role="tab"
                aria-selected={active}
                aria-controls={`panel-body-${tab.id}`}
                aria-keyshortcuts="Alt+ArrowLeft Alt+ArrowRight"
                tabIndex={active ? 0 : -1}
                title={tab.kind === 'file' ? tab.file && isMarkdownPath(tab.file.path) ? title : tab.file?.path || title : target?.displayUrl || title}
                onTap={() => onSelect(tab.id)}
                onKeyDown={(event) => {
                  if (event.altKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
                    event.preventDefault()
                    const destination = index + (event.key === 'ArrowLeft' ? -1 : 1)
                    if (destination >= 0 && destination < tabs.length) {
                      const ids = tabs.map((tab) => tab.id)
                      ids.splice(destination, 0, ids.splice(index, 1)[0])
                      onReorder(ids)
                      requestAnimationFrame(() => document.getElementById(`panel-tab-${tab.id}`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
                    }
                    return
                  }
                  const next = event.key === 'ArrowRight' ? (index + 1) % tabs.length
                    : event.key === 'ArrowLeft' ? (index + tabs.length - 1) % tabs.length
                      : event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : -1
                  if (next >= 0) {
                    event.preventDefault()
                    focusTab(tabs[next].id)
                  } else if (event.key === 'Delete') {
                    event.preventDefault()
                    closeTabs([tab])
                  } else if (event.key === ' ') {
                    event.preventDefault()
                    onSelect(tab.id)
                  }
                }}
                className="flex h-7 min-w-0 max-w-44 touch-none cursor-grab items-center gap-1.5 rounded-lg pl-2 pr-1 text-xs active:cursor-grabbing pointer-coarse:h-10"
              >
                {tab.kind === 'preview' ? <Favicon url={target?.displayUrl || ''} iconUrl={target?.favicon} />
                  : tab.kind === 'app' ? <span className="text-ink-3"><AppIcon app={tab.app} size={14} /></span>
                    : tab.kind === 'view' ? <LayoutDashboard size={14} className="shrink-0 text-ink-3" aria-hidden />
                      : <KindIcon kind={tab.kind} path={tab.kind === 'file' ? tab.file?.path : undefined} />}
                <span className="truncate">{title}</span>
              </motion.button>
              <button type="button" aria-label={`Close ${title}`} title={`Close ${title}`} onPointerDownCapture={(event) => event.stopPropagation()} onClick={() => closeTabs([tab])} className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-lg pointer-coarse:size-10 text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink">
                <X size={12} />
              </button>
            </Reorder.Item>
          )
        })}
      </Reorder.Group>
      <SidePanelTabMenu sideChatAvailable={sideChatAvailable} onAdd={onAdd} />
      {menu && menuTab ? (
        <ContextMenu point={menu.point} onClose={() => setMenu(null)}>
          <div role="menu" aria-label="Tab actions">
            {menuTab.kind === 'preview' ? <>
              {menuRow(Plus, 'New tab to the right', () => onAdd('preview', menuTab.id))}
              {browsers.find((browser) => browser.id === menuTab.id)?.target.sourceUrl && menuRow(RotateCw, 'Reload', () => sessions.reload(menuTab.id))}
              {menuRow(Copy, 'Duplicate', () => onDuplicate(menuTab.id))}
              <div className="my-1 h-px bg-border/70" />
            </> : null}
            {menuPath && <CopyPathMenuRow path={menuPath} onClose={() => setMenu(null)} />}
            {menuRow(X, 'Close', () => closeTabs([menuTab]))}
            {tabs.length > 1 && menuRow(X, 'Close other tabs', () => closeTabs(tabs.filter((tab) => tab !== menuTab)))}
            {menuIndex < tabs.length - 1 && menuRow(X, 'Close tabs to the right', () => closeTabs(tabs.slice(menuIndex + 1)))}
          </div>
        </ContextMenu>
      ) : null}
    </div>
  )
}
