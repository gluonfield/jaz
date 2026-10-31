import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, type SyntheticEvent } from 'react'
import { ContextMenu, MenuRow } from '@/components/ui/Popover'
import { Select } from '@/components/ui/Select'
import { useToast } from '@/components/ui/toast'
import type { CanvasPage } from '@/lib/api/canvas'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'

export function CanvasPages({ pages, page, onPage, onCreate, onRename, onDelete }: {
  pages: CanvasPage[]
  page: CanvasPage
  onPage: (id: string) => void
  onCreate: () => Promise<void>
  onRename: (name: string) => void
  onDelete: () => Promise<void>
}) {
  const toast = useToast()
  const [name, setName] = useState<string | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const { onClick: suppressClick, ...menuTriggers } = useContextMenuTrigger(setMenu)
  const stopPortalEvents = (event: SyntheticEvent) => {
    if (!event.currentTarget.contains(event.target as Node)) event.stopPropagation()
  }
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
    const value = name?.trim()
    if (value && value !== page.name) onRename(value)
    setName(null)
  }
  return (
    <>
      <div
        className="canvas-pages canvas-overlay"
        aria-label="Canvas pages"
        {...(name === null ? menuTriggers : {})}
        onClickCapture={suppressClick}
        onContextMenuCapture={stopPortalEvents}
        onTouchStartCapture={stopPortalEvents}
        onKeyDown={(event) => {
          if (!event.currentTarget.contains(event.target as Node) || name !== null || !(event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))) return
          event.preventDefault()
          const rect = event.currentTarget.getBoundingClientRect()
          setMenu({ x: rect.left, y: rect.bottom })
        }}
      >
        {name === null ? <Select aria-label="Canvas page" value={page.id} options={pages.map((page) => ({ value: page.id, label: page.name }))} onChange={onPage} /> : (
          <input
            autoFocus
            aria-label="Page name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            onBlur={rename}
            onKeyDown={(event) => {
              if (event.key === 'Enter') rename()
              if (event.key === 'Escape') setName(null)
              event.stopPropagation()
            }}
          />
        )}
        <button type="button" aria-label="Add page" title="Add page" disabled={saving} onClick={() => void save(onCreate)}><Plus size={16} /></button>
      </div>
      {menu ? <ContextMenu point={menu} onClose={() => setMenu(null)}>
        <div role="menu" aria-label={`${page.name} actions`}>
          <MenuRow disabled={saving} onClick={() => {
            setMenu(null)
            setName(page.name)
          }}><span className="flex items-center gap-2"><Pencil size={13} />Rename page</span></MenuRow>
          <MenuRow disabled={saving || pages.length === 1} onClick={() => void save(onDelete)}><span className="flex items-center gap-2 text-danger"><Trash2 size={13} />Delete page</span></MenuRow>
          {pages.length === 1 ? <p className="px-2.5 py-1 text-xs text-ink-3">Keep at least one page.</p> : null}
        </div>
      </ContextMenu> : null}
    </>
  )
}
