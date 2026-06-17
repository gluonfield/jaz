import { Check, LayoutGrid } from 'lucide-react'
import type { Board } from '@/lib/api/types'

export function BoardAssignmentPicker({
  boards,
  selected,
  disabled,
  onChange,
  hint,
}: {
  boards: Board[]
  selected: string[]
  disabled?: boolean
  onChange: (boardIds: string[]) => void
  hint?: string
}) {
  const selectedSet = new Set(selected)
  const toggle = (id: string) =>
    onChange(selectedSet.has(id) ? selected.filter((b) => b !== id) : [...selected, id])

  return (
    <div>
      <div className="flex flex-wrap items-center gap-1.5">
        {boards.map((board) => {
          const active = selectedSet.has(board.id)
          return (
            <button
              key={board.id}
              type="button"
              disabled={disabled}
              aria-pressed={active}
              onClick={() => toggle(board.id)}
              className={`flex h-8 min-w-0 items-center gap-1.5 rounded-full px-3 text-[12px] font-medium ring-1 transition duration-150 active:scale-[0.96] disabled:opacity-50 ${
                active
                  ? 'bg-primary-soft text-primary-strong ring-primary/20 shadow-sm'
                  : 'text-ink-2 ring-border hover:bg-surface hover:text-ink'
              }`}
            >
              {active ? <Check size={12} /> : <LayoutGrid size={12} />}
              <span className="truncate">{board.name}</span>
            </button>
          )
        })}
      </div>
      {hint ? <span className="mt-1.5 block text-[12px] text-ink-3">{hint}</span> : null}
    </div>
  )
}
