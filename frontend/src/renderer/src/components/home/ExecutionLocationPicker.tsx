import { ChevronDown, Cloud, Laptop, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { ConnectedMachine } from '@/lib/api/serverTypes'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudSession } from '@/lib/cloudSession'

export type ExecutionLocation = 'local' | 'cloud'

export function ExecutionLocationPicker({
  location,
  machines = [],
  lockedCloud = false,
  placement,
  disabled,
  onChange,
}: {
  location: ExecutionLocation
  machines?: ConnectedMachine[]
  lockedCloud?: boolean
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (location: ExecutionLocation) => void
}) {
  const cloud = useCloudSession()
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)

  const showLocal = !lockedCloud && clientRuntime.capabilities.localBackend
  const showCloud = cloud.active || !showLocal || cloud.workspaces.length === 0
  const choices = [
    ...(showLocal ? [{ id: 'local' as const, label: cloud.active ? 'This machine' : 'Local' }] : []),
    ...(showCloud ? [{ id: 'cloud' as const, label: 'Cloud' }] : []),
  ]
  const selected = choices.find(({ id }) => id === location) ?? choices[0]
  const { label } = selected
  const Icon = selected.id === 'cloud' ? Cloud : Laptop
  const peers = cloud.active ? machines : []

  return (
    <>
      {choices.length === 1 && peers.length === 0 ? (
        <span className="inline-flex h-7 items-center gap-1.5 px-2.5 text-[12px] font-medium text-ink-2">
          <Icon size={13} className="shrink-0" />
          {label}
        </span>
      ) : (
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
            {choices.map(({ id, label }) => (
              <MenuRow
                key={id}
                role="menuitemradio"
                selected={selected.id === id}
                trailing={id === 'cloud' && cloud.status === 'checking' ? (
                  <LoaderCircle size={12} className="shrink-0 animate-spin text-ink-3" />
                ) : null}
                onClick={() => {
                  setOpen(false)
                  onChange(id)
                  if (id === 'cloud' && !cloud.active) setAdding(true)
                }}
              >
                {label}
              </MenuRow>
            ))}
            {peers.map((machine) => (
              <MenuRow
                key={machine.device_id}
                role="menuitemradio"
                disabled
                onClick={() => {}}
                trailing={<span className="text-ink-3">Available to Cloud tools</span>}
              >
                {machine.display_name}
              </MenuRow>
            ))}
          </div>
        </Popover>
      )}
      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </>
  )
}
