import { ChevronsUpDown, Laptop, LoaderCircle, Plus } from 'lucide-react'
import { useRef, useState } from 'react'
import { AddCloudWorkspaceModal } from '@/components/connection/AddCloudWorkspaceModal'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import { type CloudWorkspaceSummary, LOCAL_WORKSPACE_ID, switchWorkspace } from '@/lib/cloudAccount'
import { useCloudSession } from '@/lib/cloudSession'
import { activeCloudWorkspace, workspaceDisplayName } from '@/lib/cloudWorkspaceDisplay'

const DEFAULT_CLASS_NAME = 'shrink-0 pl-1.5 pr-3 pt-1.5 pb-1 max-sm:px-4 max-sm:pt-2'
const IDENTITY_CLASS =
  'flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left text-[13px] font-semibold text-ink max-sm:h-11 max-sm:px-3 max-sm:text-[15px]'

// A Cloud workspace shows its initial; Local shows the device.
function WorkspaceMark({ workspace }: { workspace: CloudWorkspaceSummary | null }) {
  return (
    <span
      className={`grid size-5 shrink-0 place-items-center rounded-md text-[11px] font-semibold uppercase ${
        workspace ? 'bg-ink text-bg' : 'bg-list-active text-ink-2'
      }`}
    >
      {workspace ? workspaceDisplayName(workspace).charAt(0) : <Laptop size={12} />}
    </span>
  )
}

function WorkspaceLabel({ workspace }: { workspace: CloudWorkspaceSummary | null }) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2">
      <WorkspaceMark workspace={workspace} />
      <span className="min-w-0 truncate">{workspace ? workspaceDisplayName(workspace) : 'Local'}</span>
    </span>
  )
}

// The main sidebar shows the switcher once a Cloud workspace is connected;
// Settings always shows it, so the first Cloud workspace can be added there.
export function SidebarWorkspaceSwitcher({
  className = DEFAULT_CLASS_NAME,
  showWithoutCloud = false,
}: {
  className?: string
  showWithoutCloud?: boolean
}) {
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
        <p className={IDENTITY_CLASS}>
          <WorkspaceLabel workspace={activeCloud} />
        </p>
      </div>
    ) : null
  }
  if (workspaces.length === 0 && !showWithoutCloud) return null

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
            className={`group/workspace ${IDENTITY_CLASS} cursor-pointer transition-colors duration-150 hover:bg-list-hover aria-expanded:bg-list-hover aria-disabled:cursor-default aria-disabled:opacity-60`}
          >
            <WorkspaceLabel workspace={activeCloud} />
            {switchingTo !== null ? (
              <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" />
            ) : (
              <ChevronsUpDown
                size={13}
                className="shrink-0 text-ink-3 transition-colors duration-150 group-hover/workspace:text-ink-2 group-aria-expanded/workspace:text-ink-2"
              />
            )}
          </button>
        }
      >
        <div
          role="menu"
          aria-label="Switch workspace"
          className="w-[232px]"
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
              <WorkspaceLabel workspace={ws} />
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
                <WorkspaceLabel workspace={null} />
              </MenuRow>
              <div className="mx-2.5 my-1.5 h-px bg-border" />
              <MenuRow
                role="menuitem"
                onClick={() => {
                  closeMenu()
                  setAdding(true)
                }}
              >
                <span className="flex items-center gap-2">
                  <span className="grid size-5 shrink-0 place-items-center">
                    <Plus size={14} />
                  </span>
                  Add Cloud Workspace
                </span>
              </MenuRow>
            </>
          ) : null}
        </div>
      </Popover>
      <AddCloudWorkspaceModal open={adding} onClose={() => setAdding(false)} />
    </div>
  )
}
