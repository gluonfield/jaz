import { ChevronDown, Cloud, Laptop } from 'lucide-react'
import { useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { ConnectedMachine } from '@/lib/api/serverTypes'

export type ExecutionLocation = 'local' | 'cloud'

const labels: Record<ExecutionLocation, string> = { local: 'This machine', cloud: 'Cloud' }

export function ExecutionLocationPicker({
  location,
  localAvailable,
  machines,
  placement,
  disabled,
  onChange,
}: {
  location: ExecutionLocation
  localAvailable: boolean
  machines: ConnectedMachine[]
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (location: ExecutionLocation) => void
}) {
  const [open, setOpen] = useState(false)
  const label = labels[location]
  const Icon = location === 'cloud' ? Cloud : Laptop
  if (!localAvailable && machines.length === 0) {
    return (
      <span className="inline-flex h-7 items-center gap-1.5 px-2.5 text-[12px] font-medium text-ink-2">
        <Icon size={13} className="shrink-0" />
        {label}
      </span>
    )
  }
  const choices: ExecutionLocation[] = localAvailable ? ['local', 'cloud'] : ['cloud']

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
          aria-label={`Work in: ${label}`}
          onClick={() => setOpen((value) => !value)}
          className="inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96] disabled:cursor-default disabled:opacity-50"
        >
          <Icon size={13} className="shrink-0" />
          {label}
          <ChevronDown size={11} className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''}`} />
        </button>
      }
    >
      <div role="menu" aria-label="Work in">
        {choices.map((id) => (
          <MenuRow
            key={id}
            role="menuitemradio"
            selected={location === id}
            onClick={() => {
              setOpen(false)
              onChange(id)
            }}
          >
            {labels[id]}
          </MenuRow>
        ))}
      </div>
      {machines.length > 0 && (
        <div className="mt-1 border-t border-border px-2.5 pt-1.5 text-[12px]">
          <p className="text-ink-3">Available to Cloud tools</p>
          <ul aria-label="Machines available to Cloud tools" className="max-h-32 overflow-y-auto py-1 text-ink-2">
            {machines.map((machine) => (
              <li key={machine.device_id} className="truncate py-0.5" title={machine.display_name}>{machine.display_name}</li>
            ))}
          </ul>
        </div>
      )}
    </Popover>
  )
}
