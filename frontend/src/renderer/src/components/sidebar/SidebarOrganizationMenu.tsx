import { ChevronRight, ListFilter, Plus } from 'lucide-react'
import { useCallback, useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { SidebarGroupBy, SidebarShowFilter, SidebarSortBy } from './sidebarSessionModel'

const LEGACY_STORAGE_KEY = 'jaz.sidebar.organization'
const VIEW_STORAGE_KEY = 'jaz.sidebar.view'

export type SidebarOrganization = SidebarGroupBy

export interface SidebarViewState {
  groupBy: SidebarGroupBy
  sortBy: SidebarSortBy
  show: SidebarShowFilter
}

export const DEFAULT_SIDEBAR_VIEW: SidebarViewState = {
  groupBy: 'project',
  sortBy: 'updated',
  show: 'all',
}

const VALID_GROUP_BY = new Set<SidebarGroupBy>(['project', 'date', 'status', 'environment', 'none'])
const VALID_SORT_BY = new Set<SidebarSortBy>(['updated', 'created'])
const VALID_SHOW = new Set<SidebarShowFilter>(['all', 'active', 'unread'])

const GROUP_BY_LABEL: Record<SidebarGroupBy, string> = {
  date: 'Date',
  status: 'Status',
  project: 'Folder',
  environment: 'Environment',
  none: 'None',
}

const SORT_BY_LABEL: Record<SidebarSortBy, string> = {
  updated: 'Last updated',
  created: 'Created',
}

const SHOW_LABEL: Record<SidebarShowFilter, string> = {
  all: 'All',
  active: 'Active',
  unread: 'Unread',
}

const SECTION_HEADING: Record<SidebarGroupBy, string> = {
  project: 'Projects',
  date: 'Recents',
  status: 'By status',
  environment: 'Environments',
  none: 'Recents',
}

export function isDefaultSidebarView(view: SidebarViewState): boolean {
  return (
    view.groupBy === DEFAULT_SIDEBAR_VIEW.groupBy &&
    view.sortBy === DEFAULT_SIDEBAR_VIEW.sortBy &&
    view.show === DEFAULT_SIDEBAR_VIEW.show
  )
}

function storedSidebarView(): SidebarViewState {
  try {
    const raw = localStorage.getItem(VIEW_STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<SidebarViewState>
      return {
        groupBy: parsed.groupBy && VALID_GROUP_BY.has(parsed.groupBy) ? parsed.groupBy : DEFAULT_SIDEBAR_VIEW.groupBy,
        sortBy: parsed.sortBy && VALID_SORT_BY.has(parsed.sortBy) ? parsed.sortBy : DEFAULT_SIDEBAR_VIEW.sortBy,
        show: parsed.show && VALID_SHOW.has(parsed.show) ? parsed.show : DEFAULT_SIDEBAR_VIEW.show,
      }
    }
    if (localStorage.getItem(LEGACY_STORAGE_KEY) === 'recent') {
      return { ...DEFAULT_SIDEBAR_VIEW, groupBy: 'none' }
    }
  } catch {
    // Storage unavailable; fall back to defaults.
  }
  return DEFAULT_SIDEBAR_VIEW
}

export function useSidebarView(): [SidebarViewState, (next: SidebarViewState) => void] {
  const [view, setView] = useState<SidebarViewState>(storedSidebarView)
  const update = useCallback((next: SidebarViewState) => {
    setView(next)
    try {
      localStorage.setItem(VIEW_STORAGE_KEY, JSON.stringify(next))
    } catch {
      // The in-memory choice still works when storage is unavailable.
    }
  }, [])
  return [view, update]
}

type SubmenuKey = 'groupBy' | 'sortBy' | 'show'

const SUBMENU_PANEL_CLASS =
  'absolute left-full top-0 ml-1.5 min-w-[152px] rounded-[14px] bg-surface p-1.5 shadow-xl ring-1 ring-border max-sm:static max-sm:ml-2 max-sm:mt-1 max-sm:rounded-lg max-sm:bg-surface-2/60 max-sm:shadow-none max-sm:ring-0'

export function SidebarOrganizationMenu({
  view,
  onChange,
  showEnvironmentOption = false,
  onCreateProject,
}: {
  view: SidebarViewState
  onChange: (next: SidebarViewState) => void
  showEnvironmentOption?: boolean
  onCreateProject?: () => void
}) {
  const [open, setOpen] = useState(false)
  const [submenu, setSubmenu] = useState<SubmenuKey | null>(null)
  const close = useCallback(() => {
    setOpen(false)
    setSubmenu(null)
  }, [])
  const isDefault = isDefaultSidebarView(view)

  const selectGroupBy = (groupBy: SidebarGroupBy) => {
    onChange({ ...view, groupBy })
    close()
  }
  const selectSortBy = (sortBy: SidebarSortBy) => {
    onChange({ ...view, sortBy })
    close()
  }
  const selectShow = (show: SidebarShowFilter) => {
    onChange({ ...view, show })
    close()
  }

  return (
    <div className="group/organization flex h-[30px] items-center justify-between pl-2.5 pr-1 max-sm:h-11 max-sm:pl-3">
      <p className="text-[13px] font-medium text-ink-3 max-sm:text-[15px]">
        {SECTION_HEADING[view.groupBy]}
      </p>
      <div className="flex items-center gap-0.5">
        {onCreateProject ? (
          <button
            type="button"
            aria-label="New project"
            title="New project"
            onClick={onCreateProject}
            className="grid size-6 cursor-pointer place-items-center rounded-md text-ink-3 opacity-75 transition-[background-color,color,opacity] duration-150 hover:bg-list-hover hover:text-ink hover:opacity-100 focus-visible:opacity-100 group-hover/organization:opacity-100 max-sm:size-9"
          >
            <Plus size={14} />
          </button>
        ) : null}
        <Popover
          open={open}
          onClose={close}
          placement="below"
          align="end"
          followAnchor
          trigger={
            <button
              type="button"
              aria-haspopup="menu"
              aria-expanded={open}
              aria-label="Organize sidebar"
              title="Organize sidebar"
              onClick={() => {
                setOpen((value) => !value)
                setSubmenu(null)
              }}
              className={`relative grid size-6 cursor-pointer place-items-center rounded-md transition-[background-color,color,opacity,scale] duration-150 hover:bg-list-hover hover:text-ink active:scale-[0.96] focus-visible:ring-2 focus-visible:ring-primary/40 max-sm:size-9 ${
                !isDefault
                  ? 'text-primary opacity-100'
                  : 'text-ink-3 opacity-75 hover:opacity-100 group-hover/organization:opacity-100'
              }`}
            >
              <ListFilter size={14} />
              {!isDefault ? (
                <span
                  aria-hidden
                  className="absolute right-1 top-1 size-1.5 rounded-full bg-primary"
                />
              ) : null}
            </button>
          }
        >
          <div className="min-w-[196px]">
            <div
              className="relative"
              onMouseEnter={() => setSubmenu('groupBy')}
            >
              <MenuRow
                active={submenu === 'groupBy'}
                onClick={() => setSubmenu((current) => (current === 'groupBy' ? null : 'groupBy'))}
                trailing={
                  <span className="flex items-center gap-1 text-[12px] text-ink-3">
                    <span>{GROUP_BY_LABEL[view.groupBy]}</span>
                    <ChevronRight size={13} />
                  </span>
                }
              >
                Group by
              </MenuRow>
              {submenu === 'groupBy' ? (
                <div className={SUBMENU_PANEL_CLASS}>
                  <MenuRow selected={view.groupBy === 'date'} onClick={() => selectGroupBy('date')}>
                    Date
                  </MenuRow>
                  <MenuRow selected={view.groupBy === 'status'} onClick={() => selectGroupBy('status')}>
                    Status
                  </MenuRow>
                  <MenuRow selected={view.groupBy === 'project'} onClick={() => selectGroupBy('project')}>
                    Folder
                  </MenuRow>
                  {showEnvironmentOption ? (
                    <MenuRow selected={view.groupBy === 'environment'} onClick={() => selectGroupBy('environment')}>
                      Environment
                    </MenuRow>
                  ) : null}
                  <div className="my-1 border-t border-border/70" />
                  <MenuRow selected={view.groupBy === 'none'} onClick={() => selectGroupBy('none')}>
                    None
                  </MenuRow>
                </div>
              ) : null}
            </div>

            <div
              className="relative"
              onMouseEnter={() => setSubmenu('sortBy')}
            >
              <MenuRow
                active={submenu === 'sortBy'}
                onClick={() => setSubmenu((current) => (current === 'sortBy' ? null : 'sortBy'))}
                trailing={
                  <span className="flex items-center gap-1 text-[12px] text-ink-3">
                    <span>{SORT_BY_LABEL[view.sortBy]}</span>
                    <ChevronRight size={13} />
                  </span>
                }
              >
                Sort by
              </MenuRow>
              {submenu === 'sortBy' ? (
                <div className={SUBMENU_PANEL_CLASS}>
                  <MenuRow selected={view.sortBy === 'updated'} onClick={() => selectSortBy('updated')}>
                    Last updated
                  </MenuRow>
                  <MenuRow selected={view.sortBy === 'created'} onClick={() => selectSortBy('created')}>
                    Created
                  </MenuRow>
                </div>
              ) : null}
            </div>

            <div
              className="relative"
              onMouseEnter={() => setSubmenu('show')}
            >
              <MenuRow
                active={submenu === 'show'}
                onClick={() => setSubmenu((current) => (current === 'show' ? null : 'show'))}
                trailing={
                  <span className="flex items-center gap-1 text-[12px] text-ink-3">
                    <span>{SHOW_LABEL[view.show]}</span>
                    <ChevronRight size={13} />
                  </span>
                }
              >
                Show
              </MenuRow>
              {submenu === 'show' ? (
                <div className={SUBMENU_PANEL_CLASS}>
                  <MenuRow selected={view.show === 'all'} onClick={() => selectShow('all')}>
                    All
                  </MenuRow>
                  <MenuRow selected={view.show === 'active'} onClick={() => selectShow('active')}>
                    Active
                  </MenuRow>
                  <MenuRow selected={view.show === 'unread'} onClick={() => selectShow('unread')}>
                    Unread
                  </MenuRow>
                </div>
              ) : null}
            </div>

            {onCreateProject ? (
              <div onMouseEnter={() => setSubmenu(null)}>
                <div className="my-1 border-t border-border/70" />
                <MenuRow
                  onClick={() => {
                    close()
                    onCreateProject()
                  }}
                >
                  New project
                </MenuRow>
              </div>
            ) : null}

            <div onMouseEnter={() => setSubmenu(null)}>
              <div className="my-1 border-t border-border/70" />
              <MenuRow
                disabled={isDefault}
                onClick={() => {
                  onChange(DEFAULT_SIDEBAR_VIEW)
                  close()
                }}
              >
                Reset to defaults
              </MenuRow>
            </div>
          </div>
        </Popover>
      </div>
    </div>
  )
}
