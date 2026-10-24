import { Copy, FileSpreadsheet, FileText, FolderGit2, Globe, MessageCirclePlus, Plus, RotateCw, Terminal, X, type LucideIcon } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { motion, Reorder } from 'motion/react'
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { IconButton } from '@/components/ui/IconButton'
import { ContextMenu, MenuRow, Popover } from '@/components/ui/Popover'
import { Favicon } from '@/components/ui/Favicon'
import { isSpreadsheetPath } from '@shared/fileReader'
import { entrypointKey, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { useBrowserSessions } from '@/lib/browserSessions'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import type { NewSidePanelTab, SidePanelTab } from '@/lib/sidePanelTabs'

const TAB_TYPES = {
  file: { label: 'Files', icon: FileText },
  'side-chat': { label: 'Side chat', icon: MessageCirclePlus },
  preview: { label: 'Browser', icon: Globe },
  terminal: { label: 'Terminal', icon: Terminal },
  diff: { label: 'Code diff', icon: FolderGit2 },
}

function KindIcon({ kind }: { kind: keyof typeof TAB_TYPES }) {
  const Icon = TAB_TYPES[kind].icon
  return <Icon size={14} className="shrink-0 text-ink-3" aria-hidden />
}

// The new-tab menu lists the built-in tabs, then the thread apps of connected
// MCP servers.
export function SidePanelTabMenu({ sideChatAvailable, onAdd, empty = false }: {
  sideChatAvailable: boolean
  onAdd: (tab: NewSidePanelTab) => void
  empty?: boolean
}) {
  const [open, setOpen] = useState(false)
  const apps = (useQuery(mcpEntrypointsQuery).data ?? []).filter((entry) => entry.type === 'thread')
  const options = (Object.keys(TAB_TYPES) as (keyof typeof TAB_TYPES)[]).filter((kind) => kind !== 'side-chat' || sideChatAvailable)
  const row = (key: string, tab: NewSidePanelTab, icon: ReactNode, label: string) => (
    <button type="button" className="flex h-7 w-full cursor-pointer items-center gap-2 rounded-lg px-2 text-left text-[13px] text-ink-2 hover:bg-surface-2 hover:text-ink pointer-coarse:h-10" key={key} onClick={() => {
      onAdd(tab)
      setOpen(false)
    }}>
      <span className="grid size-3.5 shrink-0 place-items-center text-ink-3">{icon}</span>
      {label}
    </button>
  )
  const rows = [
    ...options.map((kind) => {
      const { label, icon: Icon } = TAB_TYPES[kind]
      return row(kind, kind, <Icon size={14} />, label)
    }),
    ...apps.map((app) => row(entrypointKey(app), app, <AppIcon app={app} size={14} />, app.title)),
  ]
  if (empty) {
    return <div className="m-auto w-52 p-2">{rows}</div>
  }
  return (
    <Popover open={open} onClose={() => setOpen(false)} placement="below" align="end" trigger={
      <IconButton size="sm" className="pointer-coarse:size-10" aria-label="New tab" aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((value) => !value)}>
        <Plus size={15} />
      </IconButton>
    }>
      {rows}
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
  const menuTab = tabs.find((tab) => tab.id === menu?.id)
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
  const menuRow = (Icon: LucideIcon, label: string, action: () => void, disabled = false) => (
    <MenuRow disabled={disabled} onClick={() => {
      setMenu(null)
      action()
    }}>
      <span className="flex items-center gap-2">
        <Icon size={13} />
        {label}
      </span>
    </MenuRow>
  )
  return (
    <div className="flex h-9 min-w-0 flex-1 items-center gap-1 px-1 pointer-coarse:h-11">
      <Reorder.Group as="div" axis="x" values={tabs.map((tab) => tab.id)} onReorder={onReorder} layoutScroll ref={list} role="tablist" aria-label="Side panel tabs" className="flex min-w-0 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden [-webkit-app-region:no-drag]">
        {tabs.map((tab, index) => {
          const label = tab.kind === 'app' ? tab.file?.name ?? tab.app.title : TAB_TYPES[tab.kind].label
          const target = browsers.find((browser) => browser.id === tab.id)?.target
          const title = tab.kind === 'file' ? tab.file?.path.split('/').pop() || label : target?.title || target?.displayUrl || label
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
                title={tab.kind === 'file' ? tab.file?.path || title : target?.displayUrl || title}
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
                    : tab.kind === 'file' && tab.file && isSpreadsheetPath(tab.file.path) ? <FileSpreadsheet size={14} className="shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden />
                      : <KindIcon kind={tab.kind} />}
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
          {menuTab.kind === 'preview' ? <>
            {menuRow(Plus, 'New tab to the right', () => onAdd('preview', menuTab.id))}
            {menuRow(RotateCw, 'Reload', () => sessions.reload(menuTab.id), !browsers.find((browser) => browser.id === menuTab.id)?.target.sourceUrl)}
            {menuRow(Copy, 'Duplicate', () => onDuplicate(menuTab.id))}
            <div className="my-1 h-px bg-border/70" />
          </> : null}
          {menuRow(X, 'Close', () => closeTabs([menuTab]))}
          {menuRow(X, 'Close other tabs', () => closeTabs(tabs.filter((tab) => tab !== menuTab)), tabs.length < 2)}
          {menuRow(X, 'Close tabs to the right', () => closeTabs(tabs.slice(tabs.indexOf(menuTab) + 1)), menuTab === tabs.at(-1))}
        </ContextMenu>
      ) : null}
    </div>
  )
}
