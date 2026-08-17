import { MoreHorizontal } from 'lucide-react'
import { useCallback, useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'

const STORAGE_KEY = 'jaz.sidebar.organization'

export type SidebarOrganization = 'project' | 'recent'

function storedOrganization(): SidebarOrganization {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'recent' ? 'recent' : 'project'
  } catch {
    return 'project'
  }
}

export function useSidebarOrganization(): [SidebarOrganization, (next: SidebarOrganization) => void] {
  const [organization, setOrganization] = useState<SidebarOrganization>(storedOrganization)
  const update = useCallback((next: SidebarOrganization) => {
    setOrganization(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // The in-memory choice still works when storage is unavailable.
    }
  }, [])
  return [organization, update]
}

export function SidebarOrganizationMenu({
  organization,
  onChange,
}: {
  organization: SidebarOrganization
  onChange: (organization: SidebarOrganization) => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="group/organization flex h-[30px] items-center justify-between pl-2.5 pr-1 max-sm:h-11 max-sm:pl-3">
      <p className="text-[13px] font-semibold text-ink max-sm:text-[15px]">
        {organization === 'project' ? 'Projects' : 'Recents'}
      </p>
      <Popover
        open={open}
        onClose={() => setOpen(false)}
        placement="below"
        align="end"
        trigger={
          <button
            type="button"
            aria-haspopup="menu"
            aria-expanded={open}
            aria-label="Organize sidebar"
            title="Organize sidebar"
            onClick={() => setOpen((value) => !value)}
            className="grid size-6 place-items-center rounded-full text-ink-3 opacity-70 transition-[background-color,color,opacity] duration-150 hover:bg-list-hover hover:text-ink hover:opacity-100 focus-visible:ring-2 focus-visible:ring-primary/40 group-hover/organization:opacity-100 max-sm:size-11"
          >
            <MoreHorizontal size={15} />
          </button>
        }
      >
        <p className="px-2.5 pb-1 pt-0.5 text-[11px] font-medium text-ink-3">Organize sidebar</p>
        <MenuRow
          selected={organization === 'project'}
          onClick={() => {
            onChange('project')
            setOpen(false)
          }}
        >
          By project
        </MenuRow>
        <MenuRow
          selected={organization === 'recent'}
          onClick={() => {
            onChange('recent')
            setOpen(false)
          }}
        >
          In one list
        </MenuRow>
      </Popover>
    </div>
  )
}
