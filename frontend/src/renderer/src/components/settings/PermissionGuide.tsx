import { GripVertical, X } from 'lucide-react'
import type { ComputerPermission } from '@shared/computerControl'
import { IconButton } from '@/components/ui/IconButton'

const APP_ICON = 'size-5 rounded-[5px] shadow-[0_0_0_0.5px_rgba(0,0,0,0.15),0_1px_2px_rgba(0,0,0,0.2)]'

export const PERMISSION_NAMES: Record<ComputerPermission, string> = { accessibility: 'Accessibility', screenRecording: 'Screen Recording' }

// The panel docked under System Settings. Its row mirrors the entry Jaz gets
// in the privacy list and drags the app bundle; the arrow above the icon
// reaches the window's bottom edge (the top of this window, 44px above the
// card) and a ghost of the icon rises along it.
export function PermissionGuide() {
  const guide = window.jaz!.permissionGuide
  const permission = window.location.hash.slice(1) as ComputerPermission
  return (
    <div className="h-full px-3 pt-11">
      <div className="rounded-2xl border border-border bg-surface p-2 shadow-[0_10px_28px_rgba(0,0,0,0.28)]">
        <div
          draggable
          onDragStart={(event) => {
            event.preventDefault()
            guide.drag()
          }}
          className="group flex h-11 cursor-grab items-center gap-2 rounded-xl border border-border bg-white pr-3 pl-1.5 font-[system-ui] shadow-[0_1px_3px_rgba(0,0,0,0.12)] transition-[translate,box-shadow] duration-150 hover:-translate-y-px hover:shadow-[0_4px_12px_rgba(0,0,0,0.2)] active:cursor-grabbing dark:bg-surface-2"
        >
          <GripVertical className="size-4 shrink-0 text-ink-3" />
          <span className="relative size-5 shrink-0">
            <svg viewBox="0 0 20 60" className="pointer-events-none absolute bottom-full left-1/2 mb-1 h-[60px] w-5 -translate-x-1/2 text-[#0a84ff]">
              <defs>
                <linearGradient id="permission-arrow" gradientUnits="userSpaceOnUse" x1="0" y1="60" x2="0" y2="0">
                  <stop offset="0" stopColor="currentColor" stopOpacity="0" />
                  <stop offset="0.6" stopColor="currentColor" />
                </linearGradient>
              </defs>
              <path d="M10 60V12" stroke="url(#permission-arrow)" strokeWidth="2.5" strokeLinecap="round" />
              <path d="M10 1 18 13H2Z" fill="currentColor" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
            </svg>
            <img src="./icon-1024.png" alt="" draggable={false} className={APP_ICON} />
            <img src="./icon-1024.png" alt="" draggable={false} className={`absolute inset-0 ${APP_ICON} opacity-0 group-hover:invisible motion-safe:animate-[permission-ghost_2.6s_ease-in-out_infinite]`} />
          </span>
          <span className="flex-1 text-[13px] text-ink">Jaz</span>
          <span aria-hidden className="flex h-[18px] w-9 items-center justify-end rounded-full bg-[#0a84ff] p-0.5">
            <span className="h-[14px] w-5 rounded-full bg-white shadow-sm" />
          </span>
        </div>
        <div className="flex items-center gap-2 pt-2 pl-1.5">
          <p className="flex-1 text-[12.5px] leading-snug text-ink-2">
            Drag <span className="font-medium text-ink">Jaz</span> into the list above to allow <span className="font-medium text-ink">{PERMISSION_NAMES[permission]}</span>
          </p>
          <IconButton size="xs" aria-label="Close" onClick={guide.close}><X className="size-3.5" /></IconButton>
        </div>
      </div>
    </div>
  )
}
