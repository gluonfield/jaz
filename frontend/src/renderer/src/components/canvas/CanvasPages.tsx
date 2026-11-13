import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useLayoutEffect, useRef, useState } from 'react'
import { ContextMenu, MenuRow } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import type { CanvasPage } from '@/lib/api/canvas'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'

// Every page as a tab, left to right. A tab's own menu renames or deletes its
// page.
export function CanvasPages({ pages, page, onPage, onCreate, onRename, onDelete }: {
  pages: CanvasPage[]
  page: CanvasPage
  onPage: (id: string) => void
  onCreate: () => Promise<void>
  onRename: (id: string, name: string) => void
  onDelete: (id: string) => Promise<void>
}) {
  const toast = useToast()
  const tabs = useRef<HTMLDivElement>(null)
  const tab = (id: string) => tabs.current?.querySelector<HTMLButtonElement>(`button[data-page="${CSS.escape(id)}"]`)
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; id: string } | null>(null)
  const { onClick: suppressClick, ...menuTriggers } = useContextMenuTrigger((point, target) => setMenu({ ...point, id: (target as HTMLElement).dataset.page! }))
  const target = pages.find((entry) => entry.id === menu?.id)
  const [saving, setSaving] = useState(false)
  const save = async (action: () => Promise<void>) => {
    setMenu(null)
    setSaving(true)
    try {
      await action()
    } catch (error) {
      toast(`Couldn't save the page: ${(error as Error).message}`, 'danger')
    } finally {
      setSaving(false)
    }
  }
  const rename = () => {
    const value = renaming?.name.trim()
    if (renaming && value && value !== pages.find((entry) => entry.id === renaming.id)?.name) onRename(renaming.id, value)
    setRenaming(null)
  }
  // The row moves only as far as shows the current tab.
  useLayoutEffect(() => {
    const row = tabs.current!
    const current = row.querySelector<HTMLElement>('[aria-current]')
    if (current) row.scrollLeft = Math.min(current.offsetLeft, Math.max(row.scrollLeft, current.offsetLeft + current.offsetWidth - row.clientWidth))
  }, [page.id])
  return (
    <>
      <div className="canvas-pages canvas-overlay" onClickCapture={suppressClick}>
        <div
          ref={tabs}
          className="canvas-page-tabs"
          role="group"
          aria-label="Canvas pages"
          onKeyDown={(event) => {
            const focused = (event.target as HTMLElement).closest<HTMLElement>('button[data-page]')
            if (!focused || !(event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))) return
            event.preventDefault()
            const rect = focused.getBoundingClientRect()
            setMenu({ x: rect.left, y: rect.bottom, id: focused.dataset.page! })
          }}
        >
          {pages.map((entry) => (renaming?.id === entry.id ? (
            <input
              key={entry.id}
              autoFocus
              aria-label="Page name"
              value={renaming.name}
              onChange={(event) => setRenaming({ id: entry.id, name: event.target.value })}
              onBlur={rename}
              onKeyDown={(event) => {
                if (event.key === 'Enter') rename()
                if (event.key === 'Escape') setRenaming(null)
                if (event.key === 'Enter' || event.key === 'Escape') requestAnimationFrame(() => tab(entry.id)?.focus())
                event.stopPropagation()
              }}
            />
          ) : (
            <button
              key={entry.id}
              type="button"
              data-page={entry.id}
              aria-current={entry.id === page.id ? 'page' : undefined}
              title={entry.name}
              {...menuTriggers}
              onClick={() => {
                setMenu(null)
                onPage(entry.id)
              }}
            >
              {entry.name}
            </button>
          )))}
        </div>
        <button type="button" aria-label="Add page" title="Add page" disabled={saving} onClick={() => void save(onCreate)}><Plus size={16} /></button>
      </div>
      {menu && target ? <ContextMenu point={menu} onClose={() => setMenu(null)}>
        <div role="menu" aria-label={`${target.name} actions`} onKeyDown={(event) => {
          if (event.key === 'Escape') {
            setMenu(null)
            tab(target.id)?.focus()
            event.stopPropagation()
          }
          if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
          event.preventDefault()
          const rows = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
          const index = rows.indexOf(document.activeElement as HTMLButtonElement)
          rows[(index + (event.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length]?.focus()
        }}>
          <MenuRow autoFocus role="menuitem" disabled={saving} onClick={() => {
            setMenu(null)
            setRenaming({ id: target.id, name: target.name })
          }}><span className="flex items-center gap-2"><Pencil size={13} />Rename page</span></MenuRow>
          <MenuRow role="menuitem" disabled={saving || pages.length === 1} onClick={() => void save(() => onDelete(target.id))}><span className="flex items-center gap-2 text-danger"><Trash2 size={13} />Delete page</span></MenuRow>
          {pages.length === 1 ? <p className="px-2.5 py-1 text-xs text-ink-3">Keep at least one page.</p> : null}
        </div>
      </ContextMenu> : null}
    </>
  )
}
