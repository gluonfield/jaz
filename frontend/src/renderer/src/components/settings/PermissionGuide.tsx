import { GripVertical, X } from 'lucide-react'
import type { ComputerPermission } from '@shared/computerControl'
import { IconButton } from '@/components/ui/IconButton'

const APP_ICON = 'size-5 rounded-[5px] shadow-[0_0_0_0.5px_rgba(0,0,0,0.15),0_1px_2px_rgba(0,0,0,0.2)]'

export const PERMISSION_NAMES: Record<ComputerPermission, string> = { accessibility: 'Accessibility', screenRecording: 'Screen Recording' }

// The panel docked to System Settings. Its row mirrors the entry Jaz gets in
// the privacy list and drags the app bundle. Beside the window (?side) the
// arrow points left from the row's height to the window's edge at this
// window's left; under it, the arrow rises from the icon to the bottom edge at
// this window's top. A ghost of the icon travels the arrow into the list.
export function PermissionGuide() {
  const guide = window.jaz!.permissionGuide
  const permission = window.location.hash.slice(1) as ComputerPermission
  const side = window.location.search === '?side'
  return (
    <div className={`relative h-full ${side ? 'py-3 pr-3 pl-12' : 'px-3 pt-11'}`}>
      {side && (
        <Arrow viewBox="0 0 44 20" className="absolute top-[43px] left-0.5 h-5 w-11 -translate-y-1/2" from={[44, 10]} to={[12, 10]} head="M1 10 13 3v14Z" />
      )}
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
            {!side && (
              <Arrow viewBox="0 0 20 60" className="absolute bottom-full left-1/2 mb-1 h-[60px] w-5 -translate-x-1/2" from={[10, 60]} to={[10, 12]} head="M10 1 18 13H2Z" />
            )}
            <img src="./icon-1024.png" alt="" draggable={false} className={APP_ICON} />
            <img
              src="./icon-1024.png"
              alt=""
              draggable={false}
              className={`absolute inset-0 ${APP_ICON} ${side ? '[--ghost-x:-92px] [--ghost-y:0px]' : '[--ghost-x:0px] [--ghost-y:-66px]'} opacity-0 group-hover:invisible motion-safe:animate-[permission-ghost_2.6s_ease-in-out_infinite]`}
            />
          </span>
          <span className="flex-1 text-[13px] text-ink">Jaz</span>
          <span aria-hidden className="flex h-[18px] w-9 items-center justify-end rounded-full bg-[#0a84ff] p-0.5">
            <span className="h-[14px] w-5 rounded-full bg-white shadow-sm" />
          </span>
        </div>
        <div className="flex items-center gap-2 pt-2 pl-1.5">
          <p className="flex-1 text-[12.5px] leading-snug">
            <span className="block font-medium text-ink">Drag Jaz into the list</span>
            <span className="block text-ink-2">to allow {PERMISSION_NAMES[permission]}</span>
          </p>
          <IconButton size="xs" aria-label="Close" onClick={guide.close}><X className="size-3.5" /></IconButton>
        </div>
      </div>
    </div>
  )
}

// A macOS-blue arrow whose shaft fades in from its tail.
function Arrow({ viewBox, className, from, to, head }: { viewBox: string; className: string; from: [number, number]; to: [number, number]; head: string }) {
  return (
    <svg viewBox={viewBox} className={`pointer-events-none text-[#0a84ff] ${className}`}>
      <defs>
        <linearGradient id="permission-arrow" gradientUnits="userSpaceOnUse" x1={from[0]} y1={from[1]} x2={to[0]} y2={to[1]}>
          <stop offset="0" stopColor="currentColor" stopOpacity="0" />
          <stop offset="0.6" stopColor="currentColor" />
        </linearGradient>
      </defs>
      <path d={`M${from.join(' ')}L${to.join(' ')}`} stroke="url(#permission-arrow)" strokeWidth="2.5" strokeLinecap="round" />
      <path d={head} fill="currentColor" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  )
}
