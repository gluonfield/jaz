import { MoreHorizontal } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'

export type MenuAction = { label: string; hint?: string; onSelect: () => void }

// A "…" button opening a menu of actions, named for what the actions act on.
export function ActionMenu({ label, actions, className }: { label: string; actions: MenuAction[]; className?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <Popover open={open} onClose={() => setOpen(false)} placement="below" align="end" trigger={
      <IconButton size="sm" aria-label={label} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((value) => !value)} className={className}>
        <MoreHorizontal size={15} />
      </IconButton>
    }>
      <div role="menu" aria-label={label}>
        {actions.map((action) => (
          <MenuRow key={action.label} role="menuitem" trailing={action.hint ? <span className="max-w-56 truncate text-[12px] text-ink-3">{action.hint}</span> : null} onClick={() => {
            setOpen(false)
            action.onSelect()
          }}>
            {action.label}
          </MenuRow>
        ))}
      </div>
    </Popover>
  )
}
