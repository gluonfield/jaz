import { ChevronDown, Cloud, Laptop, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudSession } from '@/lib/cloudSession'

export type ExecutionLocation = 'local' | 'cloud'

export function ExecutionLocationPicker({
  location,
  lockedCloud = false,
  placement,
  disabled,
  onChange,
}: {
  location: ExecutionLocation
  lockedCloud?: boolean
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (location: ExecutionLocation) => void
}) {
  const cloud = useCloudSession()
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)

  // Cloud-only packaging has no Local; the Local workspace stays strictly Local
  // while Cloud workspaces are connected, so Cloud threads never leak into it.
  const cloudOnly = lockedCloud || !clientRuntime.capabilities.localBackend
  const fixed = cloudOnly ? 'cloud' : !cloud.active && cloud.workspaces.length > 0 ? 'local' : null
  const shown = fixed ?? location
  const Icon = shown === 'local' ? Laptop : Cloud
  const label = shown === 'local' ? 'Local' : 'Cloud'

  return (
    <>
      {fixed ? (
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
            <MenuRow
              role="menuitemradio"
              selected={location === 'local'}
              onClick={() => {
                setOpen(false)
                onChange('local')
              }}
            >
              Local
            </MenuRow>
            <MenuRow
              role="menuitemradio"
              selected={location === 'cloud'}
              trailing={
                cloud.status === 'checking' ? (
                  <LoaderCircle size={12} className="shrink-0 animate-spin text-ink-3" />
                ) : null
              }
              onClick={() => {
                setOpen(false)
                onChange('cloud')
                if (!cloud.active) setAdding(true)
              }}
            >
              Cloud
            </MenuRow>
          </div>
        </Popover>
      )}
      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </>
  )
}
