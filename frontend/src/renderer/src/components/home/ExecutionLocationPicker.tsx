import { Check, ChevronDown, Cloud, Laptop, LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import { clientRuntime } from '@/lib/clientRuntime'
import { useCloudSession } from '@/lib/cloudSession'
import { describeBackend } from '@/lib/connectionDisplay'

export type ExecutionLocation = 'local' | 'cloud'

export function ExecutionLocationPicker({
  location,
  lockedCloud = false,
  placement,
  disabled,
  onChange,
  onConnectCloud,
}: {
  location: ExecutionLocation
  lockedCloud?: boolean
  placement?: 'above' | 'below'
  disabled?: boolean
  onChange: (location: ExecutionLocation) => void
  onConnectCloud: () => void
}) {
  const cloud = useCloudSession()
  const [open, setOpen] = useState(false)

  const cloudOnly = lockedCloud || !clientRuntime.capabilities.localBackend
  if (cloudOnly) {
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

  // Switched into the Local workspace while Cloud workspace(s) are connected:
  // keep Local workspace strictly Local ACP so Cloud threads never leak into Local.
  if (!cloud.active && cloud.workspaces.length > 0) {
    return (
      <span
        className="inline-flex h-7 items-center gap-1.5 rounded-full bg-surface px-2.5 text-[12px] font-medium text-ink-2"
        title="Local workspace — runs on this computer"
      >
        <Laptop size={13} className="shrink-0" />
        <span>Local</span>
      </span>
    )
  }

  const cloudSubtitle = cloud.active
    ? cloud.user?.org_name || describeBackend(cloud.url).title
    : cloud.status === 'checking'
      ? 'Connecting…'
      : 'Sign in…'

  const triggerLabel = location === 'local' ? 'Local' : 'Cloud'

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
          ) : (
            <Cloud size={13} className="shrink-0 text-primary" />
          )}
          <span className="max-w-[160px] truncate">{triggerLabel}</span>
          <ChevronDown size={11} className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''}`} />
        </button>
      }
    >
      <div role="menu" aria-label="Work in" className="w-[240px]">
        <p className="px-2.5 pb-1 pt-0.5 text-[11px] font-medium text-ink-3">Work in</p>
        <button
          type="button"
          role="menuitemradio"
          aria-checked={location === 'local'}
          onClick={() => {
            setOpen(false)
            onChange('local')
          }}
          className="flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
        >
          <Laptop size={14} className="shrink-0 text-ink-3" />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">Local</span>
            <span className="block truncate text-[11px] text-ink-3">Runs on this machine</span>
          </span>
          {location === 'local' ? <Check size={13} className="shrink-0 text-primary" /> : null}
        </button>
        <button
          type="button"
          role="menuitemradio"
          aria-checked={location === 'cloud'}
          onClick={() => {
            setOpen(false)
            if (cloud.active) {
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
          {location === 'cloud' && cloud.active ? (
            <Check size={13} className="shrink-0 text-primary" />
          ) : null}
        </button>
      </div>
    </Popover>
  )
}
