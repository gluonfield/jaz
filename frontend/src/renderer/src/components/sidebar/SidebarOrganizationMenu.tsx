import { ChevronRight, ListFilter, Plus } from 'lucide-react'
import { type ReactNode, useCallback, useState } from 'react'
import { MenuRow, Popover } from '@/components/ui/Popover'
import type { SidebarGroupBy, SidebarShowFilter, SidebarSortBy } from './sidebarSessionModel'

const LEGACY_STORAGE_KEY = 'jaz.sidebar.organization'
const VIEW_STORAGE_KEY = 'jaz.sidebar.view'

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

const GROUP_BY_LABEL: Record<SidebarGroupBy, string> = {
  date: 'Date',
  status: 'Status',
  project: 'Project',
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

function isKeyOf<T extends Record<string, string>>(map: T, value: unknown): value is keyof T {
  return typeof value === 'string' && Object.hasOwn(map, value)
}

function storedSidebarView(): SidebarViewState {
  try {
    const raw = localStorage.getItem(VIEW_STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<SidebarViewState>
      return {
        groupBy: isKeyOf(GROUP_BY_LABEL, parsed.groupBy) ? parsed.groupBy : DEFAULT_SIDEBAR_VIEW.groupBy,
        sortBy: isKeyOf(SORT_BY_LABEL, parsed.sortBy) ? parsed.sortBy : DEFAULT_SIDEBAR_VIEW.sortBy,
        show: isKeyOf(SHOW_LABEL, parsed.show) ? parsed.show : DEFAULT_SIDEBAR_VIEW.show,
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

const PRIMARY_GROUP_OPTIONS: SidebarGroupBy[] = ['date', 'status', 'project']
const SORT_OPTIONS: SidebarSortBy[] = ['updated', 'created']
const SHOW_OPTIONS: SidebarShowFilter[] = ['all', 'active', 'unread']

function SubmenuRow({
  label,
  valueLabel,
  open,
  onHover,
  onToggle,
  children,
}: {
  label: string
  valueLabel: string
  open: boolean
  onHover: () => void
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <div className="relative" onMouseEnter={onHover}>
      <MenuRow
        active={open}
        onClick={onToggle}
        trailing={
          <span className="flex items-center gap-1 text-[12px] text-ink-3">
            <span>{valueLabel}</span>
            <ChevronRight size={13} />
          </span>
        }
      >
        {label}
      </MenuRow>
      {open ? <div className={SUBMENU_PANEL_CLASS}>{children}</div> : null}
    </div>
  )
}

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

  const toggleSubmenu = (key: SubmenuKey) => {
    setSubmenu((current) => (current === key ? null : key))
  }
  const updateView = (patch: Partial<SidebarViewState>) => {
    onChange({ ...view, ...patch })
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
              className={`grid size-6 cursor-pointer place-items-center rounded-md transition-[background-color,color,opacity,scale] duration-150 hover:bg-list-hover hover:text-ink active:scale-[0.96] focus-visible:ring-2 focus-visible:ring-primary/40 max-sm:size-9 ${
                !isDefault
                  ? 'text-primary opacity-100'
                  : 'text-ink-3 opacity-75 hover:opacity-100 group-hover/organization:opacity-100'
              }`}
            >
              <ListFilter size={14} />
            </button>
          }
        >
          <div className="min-w-[196px]">
            <SubmenuRow
              label="Group by"
              valueLabel={GROUP_BY_LABEL[view.groupBy]}
              open={submenu === 'groupBy'}
              onHover={() => setSubmenu('groupBy')}
              onToggle={() => toggleSubmenu('groupBy')}
            >
              {PRIMARY_GROUP_OPTIONS.map((option) => (
                <MenuRow
                  key={option}
                  selected={view.groupBy === option}
                  onClick={() => updateView({ groupBy: option })}
                >
                  {GROUP_BY_LABEL[option]}
                </MenuRow>
              ))}
              {showEnvironmentOption ? (
                <MenuRow
                  selected={view.groupBy === 'environment'}
                  onClick={() => updateView({ groupBy: 'environment' })}
                >
                  {GROUP_BY_LABEL.environment}
                </MenuRow>
              ) : null}
              <div className="my-1 border-t border-border/70" />
              <MenuRow
                selected={view.groupBy === 'none'}
                onClick={() => updateView({ groupBy: 'none' })}
              >
                {GROUP_BY_LABEL.none}
              </MenuRow>
            </SubmenuRow>

            <SubmenuRow
              label="Sort by"
              valueLabel={SORT_BY_LABEL[view.sortBy]}
              open={submenu === 'sortBy'}
              onHover={() => setSubmenu('sortBy')}
              onToggle={() => toggleSubmenu('sortBy')}
            >
              {SORT_OPTIONS.map((option) => (
                <MenuRow
                  key={option}
                  selected={view.sortBy === option}
                  onClick={() => updateView({ sortBy: option })}
                >
                  {SORT_BY_LABEL[option]}
                </MenuRow>
              ))}
            </SubmenuRow>

            <SubmenuRow
              label="Show"
              valueLabel={SHOW_LABEL[view.show]}
              open={submenu === 'show'}
              onHover={() => setSubmenu('show')}
              onToggle={() => toggleSubmenu('show')}
            >
              {SHOW_OPTIONS.map((option) => (
                <MenuRow
                  key={option}
                  selected={view.show === option}
                  onClick={() => updateView({ show: option })}
                >
                  {SHOW_LABEL[option]}
                </MenuRow>
              ))}
            </SubmenuRow>

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
