import { X } from 'lucide-react'
import type { ComputerPermission } from '@shared/computerControl'
import { IconButton } from '@/components/ui/IconButton'

export const PERMISSION_NAMES: Record<ComputerPermission, string> = { accessibility: 'Accessibility', screenRecording: 'Screen Recording' }

// The panel docked under System Settings: Jaz's icon is a drag source for the
// app bundle, dropped into the permission list to grant it.
export function PermissionGuide() {
  const guide = window.jaz!.permissionGuide
  const permission = window.location.hash.slice(1) as ComputerPermission
  return (
    <div className="grid h-full place-items-center p-2">
      <div className="flex w-full items-center gap-3 rounded-2xl border border-border bg-surface py-2 pr-2 pl-2.5 shadow-[0_6px_16px_rgba(0,0,0,0.16)]">
        <img
          src="./icon-1024.png"
          alt="Jaz"
          onDragStart={(event) => {
            event.preventDefault()
            guide.drag()
          }}
          className="size-11 shrink-0 cursor-grab motion-safe:animate-[permission-nudge_1.6s_ease-in-out_infinite] active:cursor-grabbing"
        />
        <p className="min-w-0 flex-1 text-[13px] leading-snug">
          <span className="block font-medium text-ink">Drag Jaz into the list above</span>
          <span className="block text-ink-2">to allow {PERMISSION_NAMES[permission]}</span>
        </p>
        <IconButton size="sm" aria-label="Close" onClick={guide.close}><X className="size-4" /></IconButton>
      </div>
    </div>
  )
}
