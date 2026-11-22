import { Check, ChevronDown, Cloud, Laptop, LoaderCircle, MonitorSmartphone } from 'lucide-react'
import { useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import type { ConnectedMachine } from '@/lib/api/serverTypes'
import { useCloudSession } from '@/lib/cloudSession'
import { describeBackend } from '@/lib/connectionDisplay'

export type ExecutionLocation = 'local' | 'cloud'

export function ExecutionLocationPicker({
  location,
  machineId = '',
  machines = [],
  lockedCloud = false,
  placement,
  disabled,
  onChange,
  onSelectMachine,
  onConnectCloud,
}: {
  location: ExecutionLocation
  machineId?: string
  machines?: ConnectedMachine[]
  lockedCloud?: boolean
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (location: ExecutionLocation) => void
  onSelectMachine?: (machineId: string) => void
  onConnectCloud: () => void
}) {
  const cloud = useCloudSession()
  const [open, setOpen] = useState(false)

  const onlineMachines = cloud.active ? machines.filter((machine) => machine.online) : []
  const activeMachine = location === 'cloud' && machineId
    ? onlineMachines.find((machine) => machine.device_id === machineId)
    : undefined

  const cloudSubtitle = cloud.active
    ? cloud.user?.org_name || describeBackend(cloud.url).title
    : cloud.status === 'checking'
      ? 'Connecting…'
      : 'Sign in…'

  const triggerLabel = location === 'local'
    ? 'Local (This Mac)'
    : activeMachine
      ? activeMachine.display_name || activeMachine.device_id
      : 'Cloud'

  if (lockedCloud && onlineMachines.length === 0) {
    return (
      <span
        className="inline-flex h-7 items-center gap-1.5 rounded-full bg-surface px-2.5 text-[12px] font-medium text-ink"
        title={cloud.url || 'Jaz Cloud'}
      >
        <Cloud size={13} className="shrink-0 text-primary" />
        <span>Cloud</span>
      </span>
    )
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
          aria-label={`Work in: ${triggerLabel}`}
          onClick={() => setOpen((value) => !value)}
          className="inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96] disabled:cursor-default disabled:opacity-50"
        >
          {location === 'local' ? (
            <Laptop size={13} className="shrink-0" />
          ) : activeMachine ? (
            <MonitorSmartphone size={13} className="shrink-0 text-primary" />
          ) : (
            <Cloud size={13} className="shrink-0 text-primary" />
          )}
          <span className="max-w-[160px] truncate">{triggerLabel}</span>
          <ChevronDown size={11} className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''}`} />
        </button>
      }
    >
      <div role="menu" aria-label="Work in" className="w-[260px]">
        <p className="px-2.5 pb-1 pt-0.5 text-[11px] font-medium text-ink-3">Work in</p>
        {!lockedCloud ? (
          <button
            type="button"
            role="menuitemradio"
            aria-checked={location === 'local'}
            onClick={() => {
              setOpen(false)
              onSelectMachine?.('')
              onChange('local')
            }}
            className="flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
          >
            <Laptop size={14} className="shrink-0 text-ink-3" />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">Local (This Mac)</span>
              <span className="block truncate text-[11px] text-ink-3">Mac-only thread · local CLI</span>
            </span>
            {location === 'local' ? <Check size={13} className="shrink-0 text-primary" /> : null}
          </button>
        ) : null}
        <button
          type="button"
          role="menuitemradio"
          aria-checked={location === 'cloud' && !activeMachine}
          onClick={() => {
            setOpen(false)
            if (cloud.active) {
              onSelectMachine?.('')
              onChange('cloud')
            } else {
              onConnectCloud()
            }
          }}
          className="flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
        >
          {cloud.status === 'checking' ? (
            <LoaderCircle size={14} className="shrink-0 animate-spin text-primary" />
          ) : (
            <Cloud size={14} className="shrink-0 text-primary" />
          )}
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">Cloud</span>
            <span className="block truncate text-[11px] text-ink-3">{cloudSubtitle}</span>
          </span>
          {location === 'cloud' && cloud.active && !activeMachine ? (
            <Check size={13} className="shrink-0 text-primary" />
          ) : null}
        </button>
        {onlineMachines.map((machine) => {
          const selected = location === 'cloud' && machineId === machine.device_id
          return (
            <button
              key={machine.device_id}
              type="button"
              role="menuitemradio"
              aria-checked={selected}
              onClick={() => {
                setOpen(false)
                onSelectMachine?.(machine.device_id)
                onChange('cloud')
              }}
              className="flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
            >
              <MonitorSmartphone size={14} className="shrink-0 text-primary" />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{machine.display_name || machine.device_id}</span>
                <span className="block truncate text-[11px] text-ink-3">Connected machine</span>
              </span>
              {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
            </button>
          )
        })}
      </div>
    </Popover>
  )
}
