import { ChevronsUpDown, Laptop, LoaderCircle, Plus } from 'lucide-react'
import { useRef, useState } from 'react'
import { OrgIconMark } from '@/components/cloud/OrgIconMark'
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
// In the rail the workspace is its mark alone, filling a section tab's whole slot.
const RAIL_CLASS = 'grid size-9 place-items-center rounded-[10px]'
const RAIL_MARK_CLASS = 'size-9 rounded-[10px] text-[16px]'

// A Cloud workspace shows its org icon, or its initial; Local shows the device.
function WorkspaceMark({
  workspace,
  className = 'size-6 rounded-[7px] text-[12px]',
}: {
  workspace: CloudWorkspaceSummary | null
  className?: string
}) {
  if (workspace) {
    return <OrgIconMark name={workspaceDisplayName(workspace)} icon={workspace.org_icon} className={className} />
  }
  return (
    <span className={`grid shrink-0 place-items-center bg-ink text-bg ${className}`}>
      <Laptop className="size-[55%]" />
    </span>
  )
}

function WorkspaceLabel({ workspace }: { workspace: CloudWorkspaceSummary | null }) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2.5">
      <WorkspaceMark workspace={workspace} />
      <span className="min-w-0 truncate">{workspace ? workspaceDisplayName(workspace) : 'Local'}</span>
    </span>
  )
}

// A workspace scopes every section, so on desktop the switcher leads the nav
// rail as the workspace's mark; phones have no rail and keep it as a row. It
// appears once a Cloud workspace is connected; Settings shows it before that
// too, so the first Cloud workspace can be added there.
export function SidebarWorkspaceSwitcher({
  className,
  showWithoutCloud = false,
  rail = false,
}: {
  className?: string
  showWithoutCloud?: boolean
  rail?: boolean
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
  const activeName = activeCloud ? workspaceDisplayName(activeCloud) : 'Local'
  const wrapperClass = className ?? (rail ? 'flex flex-col items-center gap-2 after:h-px after:w-4 after:bg-border' : DEFAULT_CLASS_NAME)
  const railMark = <WorkspaceMark workspace={activeCloud} className={RAIL_MARK_CLASS} />

  // Without Local there is nothing to switch to or add around a single Cloud workspace.
  if (!localAllowed && workspaces.length <= 1) {
    return activeCloud ? (
      <div className={wrapperClass}>
        {rail ? (
          <p aria-label={`Workspace: ${activeName}`} title={activeName} data-tooltip-side="right" className={RAIL_CLASS}>
            {railMark}
          </p>
        ) : (
          <p className={IDENTITY_CLASS}>
            <WorkspaceLabel workspace={activeCloud} />
          </p>
        )}
      </div>
    ) : null
  }
  if (workspaces.length === 0 && !showWithoutCloud) return null

  const isLocalActive = localAllowed && !activeCloud
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
    <div className={wrapperClass}>
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
            {...(rail && !open ? { title: activeName, 'data-tooltip-side': 'right' } : {})}
            className={`group/workspace ${rail ? `${RAIL_CLASS} ring-ink/25 hover:ring-2 aria-expanded:ring-2 active:scale-[0.96]` : `${IDENTITY_CLASS} hover:bg-list-hover aria-expanded:bg-list-hover`} cursor-pointer transition-[background-color,box-shadow,transform] duration-150 aria-disabled:cursor-default aria-disabled:opacity-60`}
          >
            {rail ? (
              switchingTo !== null ? <LoaderCircle size={16} className="animate-spin text-ink-3" /> : railMark
            ) : (
              <>
                <WorkspaceLabel workspace={activeCloud} />
                {switchingTo !== null ? (
                  <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" />
                ) : (
                  <ChevronsUpDown
                    size={13}
                    className="shrink-0 text-ink-3 transition-colors duration-150 group-hover/workspace:text-ink-2 group-aria-expanded/workspace:text-ink-2"
                  />
                )}
              </>
            )}
          </button>
        }
      >
        <div
          role="menu"
          aria-label="Switch workspace"
          className="w-[272px]"
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
              large
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
                large
                autoFocus={isLocalActive}
                selected={isLocalActive}
                onClick={() => void handleSelect(LOCAL_WORKSPACE_ID)}
              >
                <WorkspaceLabel workspace={null} />
              </MenuRow>
              <div className="mx-2 my-1.5 h-px bg-border" />
              <MenuRow
                role="menuitem"
                large
                onClick={() => {
                  closeMenu()
                  setAdding(true)
                }}
              >
                <span className="flex items-center gap-2.5">
                  <span className="grid size-6 shrink-0 place-items-center rounded-[7px] border border-dashed border-ink/25">
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
