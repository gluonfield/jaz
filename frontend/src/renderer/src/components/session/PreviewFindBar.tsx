import { ChevronDown, ChevronUp, Search, X } from 'lucide-react'
import { IconButton } from '@/components/ui/IconButton'
import type { PreviewFindControls } from './usePreviewFindControls'

export function PreviewFindBar({ find }: { find: PreviewFindControls }) {
  if (!find.open) return null

  return (
    <div className="absolute top-2 right-2 z-dropdown flex h-10 max-w-[calc(100%-1rem)] items-center gap-1 rounded-full bg-surface px-2 shadow-[0_8px_30px_rgba(0,0,0,0.16)] ring-1 ring-border/70">
      <Search size={15} className="ml-1 shrink-0 text-ink-3" aria-hidden />
      <input
        ref={find.inputRef}
        value={find.query}
        onChange={(event) => find.setQuery(event.currentTarget.value)}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            event.preventDefault()
            find.close()
          } else if (event.key === 'Enter') {
            event.preventDefault()
            find.findAgain(!event.shiftKey)
          }
        }}
        placeholder="Find in page"
        aria-label="Find in page"
        className="h-8 w-[min(220px,42vw)] bg-transparent px-1 text-[13px] text-ink outline-none placeholder:text-ink-3"
      />
      <span className="min-w-12 text-right font-mono text-[11px] text-ink-3 tabular-nums">
        {find.query ? `${find.active}/${find.matches}` : ''}
      </span>
      <IconButton
        size="sm"
        aria-label="Previous match"
        title="Previous match"
        disabled={!find.matches}
        onClick={() => find.findAgain(false)}
      >
        <ChevronUp size={15} />
      </IconButton>
      <IconButton
        size="sm"
        aria-label="Next match"
        title="Next match"
        disabled={!find.matches}
        onClick={() => find.findAgain(true)}
      >
        <ChevronDown size={15} />
      </IconButton>
      <IconButton size="sm" aria-label="Close find" title="Close find" onClick={find.close}>
        <X size={15} />
      </IconButton>
    </div>
  )
}
