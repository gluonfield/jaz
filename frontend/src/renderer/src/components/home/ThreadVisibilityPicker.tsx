import { ChevronDown, Lock, Users } from 'lucide-react'
import { useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { CloudSpace } from '@/lib/api/cloudSpaces'

// Who can see a new Cloud thread: only its author, or one Space.
export function ThreadVisibilityPicker({
  spaceId,
  spaces,
  placement,
  disabled,
  onChange,
}: {
  spaceId: string
  spaces: CloudSpace[]
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (spaceId: string) => void
}) {
  const [open, setOpen] = useState(false)
  const Icon = spaceId ? Users : Lock
  const label = spaceId ? (spaces.find((space) => space.id === spaceId)?.name ?? 'Space') : 'Private'
  const select = (next: string) => {
    setOpen(false)
    onChange(next)
  }

  return (
    <Popover
      open={open && !disabled}
      onClose={() => setOpen(false)}
      placement={placement}
      align="start"
      trigger={
        <button
          type="button"
          disabled={disabled}
          aria-haspopup="menu"
          aria-expanded={open && !disabled}
          aria-label={`Thread visibility: ${label}`}
          onClick={() => setOpen((value) => !value)}
          className="inline-flex h-7 max-w-[13rem] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96] disabled:cursor-default disabled:opacity-50"
        >
          <Icon size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
          <ChevronDown size={11} className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''}`} />
        </button>
      }
    >
      <div role="menu" aria-label="Thread visibility" className="max-h-[280px] max-w-[280px] overflow-y-auto">
        <MenuRow role="menuitemradio" selected={spaceId === ''} onClick={() => select('')}>
          Private
        </MenuRow>
        {spaces.map((space) => (
          <MenuRow
            key={space.id}
            role="menuitemradio"
            selected={space.id === spaceId}
            onClick={() => select(space.id)}
          >
            {space.name}
          </MenuRow>
        ))}
      </div>
    </Popover>
  )
}
