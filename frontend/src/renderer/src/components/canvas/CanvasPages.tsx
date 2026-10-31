import { Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { Select } from '@/components/ui/Select'
import { useToast } from '@/components/ui/toast'
import type { CanvasPage } from '@/lib/api/canvas'

export function CanvasPages({ pages, page, onPage, onCreate, onRename }: {
  pages: CanvasPage[]
  page: CanvasPage
  onPage: (id: string) => void
  onCreate: () => Promise<void>
  onRename: (name: string) => void
}) {
  const toast = useToast()
  const [name, setName] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const rename = () => {
    const value = name?.trim()
    if (value && value !== page.name) onRename(value)
    setName(null)
  }
  return (
    <div className="canvas-pages canvas-overlay" aria-label="Canvas pages">
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
      <button type="button" aria-label="Rename page" title="Rename page" onClick={() => setName(page.name)}><Pencil size={14} /></button>
      <button type="button" aria-label="Add page" title="Add page" disabled={creating} onClick={async () => {
        setCreating(true)
        try {
          await onCreate()
        } catch (error) {
          toast(`Couldn't add the page: ${(error as Error).message}`, 'danger')
        } finally {
          setCreating(false)
        }
      }}><Plus size={16} /></button>
    </div>
  )
}
