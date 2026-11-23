import { ChevronsUpDown, LoaderCircle } from 'lucide-react'
import { useRef, useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import { LOCAL_WORKSPACE_ID, switchWorkspace } from '@/lib/cloudAccount'
import { useCloudSession } from '@/lib/cloudSession'
import { activeCloudWorkspace, workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'

const DEFAULT_CLASS_NAME = 'shrink-0 pl-1.5 pr-3 pt-1.5 pb-0.5 max-sm:px-4 max-sm:pt-2'

export function SidebarWorkspaceSwitcher({ className = DEFAULT_CLASS_NAME }: { className?: string }) {
  const cloud = useCloudSession()
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [switchingTo, setSwitchingTo] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)

  const localAllowed = clientRuntime.capabilities.localBackend && cloud.source !== 'primary'
  const workspaces = cloud.workspaces
  const activeCloud = activeCloudWorkspace(cloud)

  // Without Local there is nothing to switch to or add around a single Cloud workspace.
  if (!localAllowed && workspaces.length <= 1) {
    return activeCloud ? (
      <div className={className}>
        <p className="truncate px-2.5 text-[13px] leading-7 font-semibold text-ink max-sm:px-3 max-sm:text-[15px] max-sm:leading-11">
          {workspaceDisplayName(activeCloud)}
        </p>
      </div>
    ) : null
  }

  const isLocalActive = localAllowed && !activeCloud
  const activeName = activeCloud ? workspaceDisplayName(activeCloud) : 'Local'
  const closeMenu = () => {
    setOpen(false)
    triggerRef.current?.focus()
  }

  const handleSelect = async (workspaceId: string) => {
    closeMenu()
    const currentId = activeCloud ? activeCloud.id : LOCAL_WORKSPACE_ID
    if (workspaceId === currentId || switchingTo !== null) return
    setSwitchingTo(workspaceId)
    try {
      const err = await switchWorkspace(workspaceId)
      if (err) {
        toast(err, 'danger')
      }
    } finally {
      setSwitchingTo(null)
    }
  }

  return (
    <div className={className}>
      <Popover
        open={open}
        onClose={closeMenu}
        placement="below"
        align="start"
        trigger={
          <button
            ref={triggerRef}
            type="button"
            aria-disabled={switchingTo !== null}
            aria-haspopup="menu"
            aria-expanded={open}
            aria-label={`Workspace: ${activeName}`}
            onClick={() => {
              if (switchingTo === null) setOpen((value) => !value)
            }}
            className="group/workspace flex h-7 max-w-full cursor-pointer items-center gap-1.5 rounded-lg px-2.5 text-left text-[13px] font-semibold text-ink transition-[background-color,scale] duration-150 hover:bg-list-hover active:scale-[0.98] focus-visible:ring-2 focus-visible:ring-primary/40 aria-expanded:bg-list-hover aria-disabled:cursor-default aria-disabled:opacity-60 max-sm:h-11 max-sm:px-3 max-sm:text-[15px]"
          >
            <span className="min-w-0 truncate">{activeName}</span>
            {switchingTo !== null ? (
              <LoaderCircle size={12} className="shrink-0 animate-spin text-ink-3" />
            ) : (
              <ChevronsUpDown
                size={12}
                className="shrink-0 text-ink-3 transition-colors duration-150 group-hover/workspace:text-ink-2"
              />
            )}
          </button>
        }
      >
        <div
          role="menu"
          aria-label="Switch workspace"
          className="min-w-[200px] max-w-[280px]"
          onKeyDown={(event) => {
            if (event.key === 'Tab') closeMenu()
            if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
            event.preventDefault()
            const rows = [...event.currentTarget.querySelectorAll('button')]
            const index = rows.indexOf(document.activeElement as HTMLButtonElement)
            rows[(index + (event.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length]?.focus()
          }}
        >
          {workspaces.map((ws) => (
            <MenuRow
              key={ws.id}
              role="menuitemradio"
              autoFocus={activeCloud?.id === ws.id}
              selected={activeCloud?.id === ws.id}
              onClick={() => void handleSelect(ws.id)}
            >
              {workspaceDisplayName(ws)}
            </MenuRow>
          ))}
          {localAllowed ? (
            <>
              <MenuRow
                role="menuitemradio"
                autoFocus={isLocalActive}
                selected={isLocalActive}
                onClick={() => void handleSelect(LOCAL_WORKSPACE_ID)}
              >
                Local
              </MenuRow>
              <div className="mx-2.5 my-1.5 h-px bg-border" />
              <MenuRow
                role="menuitem"
                onClick={() => {
                  closeMenu()
                  setAdding(true)
                }}
              >
                Add Cloud Workspace
              </MenuRow>
            </>
          ) : null}
        </div>
      </Popover>
      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </div>
  )
}
