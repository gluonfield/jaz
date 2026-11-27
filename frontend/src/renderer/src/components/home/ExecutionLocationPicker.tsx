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
  executorTarget,
  onChangeExecutor,
  localMachine = false,
  lockedCloud = false,
  placement,
  disabled,
  onChange,
}: {
  location: ExecutionLocation
  machines?: ConnectedMachine[]
  executorTarget?: string
  onChangeExecutor?: (deviceId: string) => void
  localMachine?: boolean
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
  const cloudDevice = cloud.active ? machines.find((machine) => machine.device_id === executorTarget) : undefined
  const fixed = cloudOnly ? (machines.length ? null : 'cloud') : !cloud.active && cloud.workspaces.length > 0 ? 'local' : null
  const localLabel = localMachine ? 'This machine' : 'Local'
  const shown = cloudOnly ? 'cloud' : fixed ?? location
  const Icon = shown === 'local' ? Laptop : Cloud
  const label = shown === 'local' ? localLabel : cloudDevice ? `Cloud · ${cloudDevice.display_name}` : 'Cloud'

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
            {!cloudOnly && <MenuRow
              role="menuitemradio"
              selected={shown === 'local'}
              onClick={() => {
                setOpen(false)
                onChange('local')
              }}
            >
              {localLabel}
            </MenuRow>}
            <MenuRow
              role="menuitemradio"
              selected={shown === 'cloud' && !cloudDevice}
              trailing={
                cloud.status === 'checking' ? (
                  <LoaderCircle size={12} className="shrink-0 animate-spin text-ink-3" />
                ) : null
              }
              onClick={() => {
                setOpen(false)
                onChange('cloud')
                onChangeExecutor?.('')
                if (!cloud.active) setAdding(true)
              }}
            >
              Cloud
            </MenuRow>
            {machines.map((machine) => (
              <MenuRow
                key={machine.device_id}
                role="menuitemradio"
                selected={shown === 'cloud' && machine.device_id === cloudDevice?.device_id}
                onClick={() => {
                  setOpen(false)
                  onChange('cloud')
                  onChangeExecutor?.(machine.device_id)
                }}
              >
                {machine.display_name}
                <span className="ml-1 text-ink-3">· Cloud with device tools</span>
              </MenuRow>
            ))}
          </div>
        </Popover>
      )}
      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </>
  )
}
