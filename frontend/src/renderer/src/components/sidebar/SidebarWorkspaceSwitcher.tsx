import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Check, ChevronsUpDown, Laptop, LoaderCircle, Plus } from 'lucide-react'
import { useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import { LOCAL_WORKSPACE_ID, switchWorkspace } from '@/lib/cloudAccount'
import { useCloudSession } from '@/lib/cloudSession'
import {
  workspaceDisplayName,
  workspaceInitials,
  workspaceSubtitle,
} from '@/lib/cloudWorkspaceDisplay'

export function SidebarWorkspaceSwitcher({
  onAddWorkspace,
}: {
  onAddWorkspace: () => void
}) {
  const cloud = useCloudSession()
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [switchingTo, setSwitchingTo] = useState<string | null>(null)

  const localAllowed = clientRuntime.capabilities.localBackend
  const workspaces = cloud.workspaces

  // Pure Local mode: zero extra chrome.
  if (workspaces.length === 0) {
    return null
  }

  const activeCloud = cloud.active
    ? workspaces.find((ws) => ws.id === cloud.activeWorkspaceId) ?? workspaces[0]
    : null
  const isLocalActive = localAllowed && !activeCloud

  const activeName = activeCloud ? workspaceDisplayName(activeCloud) : 'Local'
  const activeAvatar = activeCloud ? (
    <span className="grid size-[18px] shrink-0 place-items-center rounded-[5px] bg-primary-soft text-[10px] font-semibold text-primary">
      {workspaceInitials(activeName)}
    </span>
  ) : (
    <span className="grid size-[18px] shrink-0 place-items-center rounded-[5px] bg-surface-2 text-ink-2">
      <Laptop size={12} />
    </span>
  )

  // Cloud-only packaging with a single primary workspace: clean static header, no Local escape hatch.
  if (!localAllowed && workspaces.length === 1) {
    return (
      <div className="shrink-0 pl-1.5 pr-3 pt-1.5 pb-0.5 max-sm:px-4 max-sm:pt-2">
        <div
          className="flex h-[30px] items-center gap-2 rounded-lg px-2.5 text-[13px] font-semibold text-ink max-sm:h-11 max-sm:px-3 max-sm:text-[15px]"
          title={activeCloud?.url || activeName}
        >
          {activeAvatar}
          <span className="min-w-0 flex-1 truncate">{activeName}</span>
        </div>
      </div>
    )
  }

  const handleSelect = async (workspaceId: string) => {
    setOpen(false)
    const currentId = activeCloud ? activeCloud.id : LOCAL_WORKSPACE_ID
    if (workspaceId === currentId || switchingTo !== null) return
    setSwitchingTo(workspaceId)
    try {
      const err = await switchWorkspace(workspaceId)
      if (err) {
        toast(err, 'danger')
        return
      }
      if (pathname.startsWith('/sessions/')) {
        void navigate({ to: '/new' })
      }
    } finally {
      setSwitchingTo(null)
    }
  }

  return (
    <div className="shrink-0 pl-1.5 pr-3 pt-1.5 pb-0.5 max-sm:px-4 max-sm:pt-2">
      <Popover
        open={open}
        onClose={() => setOpen(false)}
        placement="below"
        align="start"
        trigger={
          <button
            type="button"
            disabled={switchingTo !== null}
            aria-haspopup="menu"
            aria-expanded={open}
            aria-label={`Workspace: ${activeName}`}
            onClick={() => setOpen((value) => !value)}
            className="flex h-[30px] w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 text-left text-[13px] font-semibold text-ink transition-colors duration-150 hover:bg-list-hover focus-visible:ring-2 focus-visible:ring-primary/40 disabled:cursor-default disabled:opacity-60 max-sm:h-11 max-sm:px-3 max-sm:text-[15px]"
          >
            {activeAvatar}
            <span className="min-w-0 flex-1 truncate">{activeName}</span>
            {switchingTo !== null ? (
              <LoaderCircle size={13} className="shrink-0 animate-spin text-ink-3" />
            ) : (
              <ChevronsUpDown size={13} className="shrink-0 text-ink-3" />
            )}
          </button>
        }
      >
        <div role="menu" aria-label="Switch workspace" className="w-[244px]">
          <p className="px-2.5 pb-1 pt-0.5 text-[11px] font-medium text-ink-3">Workspaces</p>
          {workspaces.map((ws) => {
            const name = workspaceDisplayName(ws)
            const subtitle = workspaceSubtitle(ws)
            const selected = activeCloud?.id === ws.id
            return (
              <button
                key={ws.id}
                type="button"
                role="menuitemradio"
                aria-checked={selected}
                onClick={() => void handleSelect(ws.id)}
                className="flex w-full cursor-pointer items-center gap-2.5 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
              >
                <span className="grid size-6 shrink-0 place-items-center rounded-[6px] bg-primary-soft text-[11px] font-semibold text-primary">
                  {workspaceInitials(name)}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{name}</span>
                  {subtitle ? (
                    <span className="block truncate text-[11px] text-ink-3">{subtitle}</span>
                  ) : null}
                </span>
                {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
              </button>
            )
          })}

          {localAllowed ? (
            <button
              type="button"
              role="menuitemradio"
              aria-checked={isLocalActive}
              onClick={() => void handleSelect(LOCAL_WORKSPACE_ID)}
              className="flex w-full cursor-pointer items-center gap-2.5 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink transition-colors duration-150 hover:bg-surface-2"
            >
              <span className="grid size-6 shrink-0 place-items-center rounded-[6px] bg-surface-2 text-ink-2">
                <Laptop size={13} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">Local</span>
                <span className="block truncate text-[11px] text-ink-3">This computer</span>
              </span>
              {isLocalActive ? <Check size={13} className="shrink-0 text-primary" /> : null}
            </button>
          ) : null}

          {localAllowed ? (
            <>
              <div className="my-1 h-px bg-border" />
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false)
                  onAddWorkspace()
                }}
                className="flex w-full cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
              >
                <Plus size={14} className="shrink-0 text-ink-3" />
                <span className="truncate">Add Cloud workspace</span>
              </button>
            </>
          ) : null}
        </div>
      </Popover>
    </div>
  )
}
