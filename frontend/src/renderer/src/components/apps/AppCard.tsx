import type { ReactNode } from 'react'
import { ActionMenu, type MenuAction } from '@/components/ui/ActionMenu'

// One app in the library: its preview, its name and one quiet line of detail.
// Everything else an app can do sits in the menu that appears on hover.
export function AppCard({ title, detail, mark, preview, actions = [], onOpen }: {
  title: string
  detail?: string
  // A small trailing sign of the app's state, such as shared or needing review.
  mark?: ReactNode
  preview: ReactNode
  actions?: MenuAction[]
  onOpen: () => void
}) {
  return (
    <div className="group/card relative">
      <button
        type="button"
        onClick={onOpen}
        className="block w-full cursor-pointer overflow-hidden rounded-[14px] border border-border bg-surface/60 text-left transition-[border-color,transform] duration-150 hover:border-ink/20 active:scale-[0.99] motion-reduce:transition-none"
      >
        {preview}
        <span className="flex items-center gap-3 border-t border-border/70 px-3.5 py-3">
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[14px] font-medium text-ink">{title}</span>
            {detail ? <span className="mt-0.5 block truncate text-[12.5px] text-ink-3">{detail}</span> : null}
          </span>
          {mark}
        </span>
      </button>
      {actions.length ? (
        <div className="absolute right-2 top-2 opacity-0 transition-opacity duration-150 group-hover/card:opacity-100 focus-within:opacity-100 has-[[aria-expanded=true]]:opacity-100 pointer-coarse:opacity-100">
          <ActionMenu label={`Actions for ${title}`} actions={actions} className="bg-bg shadow-sm ring-1 ring-border hover:bg-surface-2" />
        </div>
      ) : null}
    </div>
  )
}
