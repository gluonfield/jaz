import { Settings } from 'lucide-react'
import type { ReactNode, PointerEvent as ReactPointerEvent } from 'react'
import { ConnectionFooterButton } from '@/components/connection/ConnectionFooterButton'
import { UpdatePanel } from '@/components/update/UpdatePanel'

// The panel beside the content, holding the current tab's list.
export function Sidebar({
  width,
  mobile = false,
  onDismiss,
  resizing,
  onResizeStart,
  onResizeReset,
  onOpenSettings,
  onOpenConnect,
  children,
}: {
  width: number
  mobile?: boolean
  onDismiss?: () => void
  resizing?: boolean
  onResizeStart: (e: ReactPointerEvent) => void
  onResizeReset: () => void
  onOpenSettings: () => void
  onOpenConnect: () => void
  children: ReactNode
}) {
  return (
    <aside
      onClick={
        mobile && onDismiss
          ? (event) => {
              if (!(event.target as HTMLElement).closest('button, input, textarea')) onDismiss()
            }
          : undefined
      }
      className="relative flex h-full shrink-0 flex-col max-sm:w-full! max-sm:bg-panel"
      style={{ width }}
    >
      {children}

      <div className="flex shrink-0 flex-col gap-0.5 border-t border-border pl-1.5 pr-3 py-1.5 empty:hidden max-sm:pl-3">
        <UpdatePanel />
        <ConnectionFooterButton onOpenConnect={onOpenConnect} />
        {mobile && (
          <button
            type="button"
            onClick={onOpenSettings}
            className="group flex w-full items-center gap-2 rounded-lg px-3 py-2 text-[15px] font-medium text-ink transition-colors duration-150 hover:bg-list-hover"
          >
            <span className="grid size-[18px] shrink-0 place-items-center">
              <Settings size={18} className="text-ink-2" />
            </span>
            <span className="flex-1 text-left">Settings</span>
          </button>
        )}
      </div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
        onPointerDown={onResizeStart}
        onDoubleClick={onResizeReset}
        className="group absolute inset-y-0 right-0 z-10 flex w-2 cursor-col-resize touch-none justify-end max-sm:hidden"
      >
        <span
          className={`h-full w-px transition-colors duration-150 group-hover:bg-primary/40 ${
            resizing ? 'bg-primary/60' : 'bg-transparent'
          }`}
        />
      </div>
    </aside>
  )
}
