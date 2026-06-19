import { Check } from 'lucide-react'
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
              className={`flex h-8 min-w-0 items-center rounded-full px-3 text-[12px] font-medium transition-[background-color,color,box-shadow,scale] duration-150 active:scale-[0.96] disabled:opacity-50 ${
                active
                  ? 'bg-primary-soft text-primary-strong shadow-sm'
                  : 'bg-surface text-ink-2 hover:bg-surface-2 hover:text-ink'
              }`}
            >
              {/* The check reveals by animating its column width 0fr → 1fr, so
                  selecting a board grows the pill smoothly instead of jumping. */}
              <span
                className={`grid overflow-hidden transition-[grid-template-columns,opacity,margin-right] duration-150 ease-out ${
                  active ? 'mr-1.5 grid-cols-[1fr] opacity-100' : 'grid-cols-[0fr] opacity-0'
                }`}
              >
                <Check size={12} className="min-w-0" />
              </span>
              <span className="truncate">{board.name}</span>
            </button>
          )
        })}
      </div>
      {hint ? <span className="mt-1.5 block text-[12px] text-ink-3">{hint}</span> : null}
    </div>
  )
}
