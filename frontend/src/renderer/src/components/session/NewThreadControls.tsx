import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowRight,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Folder,
  FolderPlus,
  GitBranch,
  Keyboard,
  LoaderCircle,
  Trash2,
} from 'lucide-react'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { ReasoningEffortSlider } from '@/components/acp/ReasoningEffortSlider'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Modal } from '@/components/ui/Modal'
import { ContextMenu, MenuRow, Popover } from '@/components/ui/Popover'
import { Select } from '@/components/ui/Select'
import { agentLabel } from '@/lib/agentLabel'
import { addProject, deleteProject, listFilesystemDirs, projectsQuery, type Project } from '@/lib/api/sessions'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import type { ReasoningEffortOption } from '@/lib/api/types'
import {
  filterModelSuggestions,
  modelSuggestionFor,
  modelSuggestionLabel,
  type ModelSuggestion,
} from '@/lib/models'
import { keys } from '@/lib/query/keys'
import { reasoningEffortLabel, REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'

// Selects the ACP agent backing a new thread.
export function RuntimeSelect({
  value,
  agents,
  disabled,
  placement,
  onChange,
}: {
  value: string
  agents: string[]
  disabled?: boolean
  placement?: 'above' | 'below'
  onChange: (runtime: string) => void
}) {
  const [open, setOpen] = useState(false)
  const label = agentLabel(value)
  const showLogo = hasAgentLogo(value)
  const select = (runtime: string) => {
    onChange(runtime)
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement={placement}
      trigger={
        <Button
          variant="secondary"
          size="sm"
          className="max-w-[12rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={`Agent: ${label}`}
          title={`Agent: ${label}`}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          {showLogo ? <AgentLogo agent={value} size={14} /> : null}
          <span className="truncate">{label}</span>
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      {agents.map((agent) => (
        <MenuRow key={agent} selected={value === agent} onClick={() => select(agent)}>
          <span className="flex min-w-0 items-center gap-2">
            {hasAgentLogo(agent) ? <AgentLogo agent={agent} size={14} /> : null}
            <span className="truncate">{agentLabel(agent)}</span>
          </span>
        </MenuRow>
      ))}
    </Popover>
  )
}

// Picks the model for a new thread: curated suggestions for the chosen
// agent/provider plus free-text entry for anything else.
export function ModelSelect({
  value,
  suggestions,
  loading,
  disabled,
  placement,
  onChange,
  providers,
  provider,
  onProviderChange,
  effort,
  effortOptions = REASONING_EFFORT_OPTIONS,
  onEffortChange,
}: {
  value: string
  suggestions: ModelSuggestion[]
  loading?: boolean
  disabled?: boolean
  placement?: 'above' | 'below'
  onChange: (model: string) => void
  providers?: { value: string; label: string }[]
  provider?: string
  onProviderChange?: (provider: string) => void
  // '' inherits the Settings > Agents default for the chosen agent/provider.
  effort?: string
  effortOptions?: ReasoningEffortOption[]
  onEffortChange?: (effort: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (open) {
      setQuery('')
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  const filtered = useMemo(() => filterModelSuggestions(suggestions, query), [suggestions, query])
  const typed = query.trim()
  const typedIsNew = typed !== '' && !suggestions.some((s) => s.value === typed)
  const label = value === '' ? 'Model' : modelSuggestionLabel(suggestions, value)
  const selectedSuggestion = modelSuggestionFor(suggestions, value)
  const effortStops = effortOptions.filter((option) => option.value !== '')
  const automaticReasoning = selectedSuggestion?.reasoning.automatic === true && effortStops.length === 0
  // An unset effort still reasons at the model's own default (e.g. Grok's "high"),
  // so surface that here — matching the slider, which anchors on default_effort too.
  const selectedEffort = (effort ?? '') || selectedSuggestion?.reasoning.default_effort || ''
  const effortValue = effortOptions.some((option) => option.value === selectedEffort) ? selectedEffort : ''
  const effortLabel = automaticReasoning ? 'Thinking' : reasoningEffortLabel(effortValue, effortOptions)
  const showEffortSlider = Boolean(onEffortChange) && effortStops.length > 1
  const reasoningDescription = automaticReasoning
    ? ', reasoning: automatic'
    : effortValue
      ? `, reasoning effort: ${effortLabel}`
      : ''
  const description = `Model: ${value === '' ? 'default' : label}${reasoningDescription}`
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement={placement}
      trigger={
        <Button
          variant="secondary"
          size="sm"
          className="max-w-[13rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={description}
          title={description}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="truncate">{label}</span>
          {automaticReasoning || effortValue ? (
            <span className="shrink-0 text-primary">{effortLabel}</span>
          ) : null}
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      <div className="w-[260px]">
        {providers && providers.length > 1 && onProviderChange ? (
          <div className="flex items-center justify-between gap-2 px-2 pt-1 pb-0.5">
            <span className="text-[11px] text-ink-3">Provider</span>
            <Select
              value={provider ?? providers[0].value}
              options={providers}
              onChange={onProviderChange}
              aria-label="Provider"
              className="min-w-[140px] max-w-[180px]"
            />
          </div>
        ) : null}
        <div className="px-1 pt-1 pb-1.5">
          <input
            ref={inputRef}
            value={query}
            placeholder="Search models…"
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && typed !== '') {
                e.preventDefault()
                onChange(typed)
              }
            }}
            className="h-7 w-full rounded-full bg-ink/10 px-2.5 text-[12px] text-ink outline-none transition-colors duration-150 placeholder:text-ink-3 focus:bg-ink/15"
          />
        </div>
        <div className={`${showEffortSlider ? 'max-h-[180px]' : 'max-h-[240px]'} overflow-y-auto`}>
          {typedIsNew ? (
            <MenuRow selected={typed === value} onClick={() => onChange(typed)}>
              Use “{typed}”
            </MenuRow>
          ) : null}
          {loading ? (
            <div className="flex h-7 items-center gap-2 px-2 text-[13px] text-ink-3">
              <LoaderCircle size={13} className="animate-spin" />
              Loading models…
            </div>
          ) : filtered.length > 0 ? (
            filtered.map((s) => (
              <MenuRow key={s.value} selected={s.value === value} onClick={() => onChange(s.value)}>
                {s.label}
              </MenuRow>
            ))
          ) : !typedIsNew ? (
            <div className="px-2 py-1 text-[13px] text-ink-3">No matching models.</div>
          ) : null}
        </div>
        {showEffortSlider && onEffortChange ? (
          <>
            <div className="my-1 border-t border-border" />
            <div className="px-3 pt-1.5 pb-2.5">
              <ReasoningEffortSlider
                options={effortStops}
                value={effortValue}
                defaultValue={selectedSuggestion?.reasoning.default_effort}
                onChange={onEffortChange}
              />
            </div>
          </>
        ) : null}
      </div>
    </Popover>
  )
}

function directoryName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts.at(-1) ?? path
}

type Crumb = { name: string; path: string; collapsed: boolean }

// Breadcrumb segments for a path, collapsing the middle of a deep path to a
// single "…" so the header fits the popover width.
function buildCrumbs(path: string): Crumb[] {
  const parts = path.split('/').filter(Boolean)
  const all = parts.map((name, index) => ({
    name,
    path: '/' + parts.slice(0, index + 1).join('/'),
    collapsed: false,
  }))
  if (all.length <= 3) return all
  return [{ name: '…', path: all[all.length - 4].path, collapsed: true }, ...all.slice(-3)]
}

export function ProjectPicker({
  value,
  disabled,
  placement,
  onChange,
}: {
  value: string
  disabled?: boolean
  placement?: 'above' | 'below'
  onChange: (path: string, git: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [menu, setMenu] = useState<{ point: { x: number; y: number }; project: Project } | null>(null)
  const [confirm, setConfirm] = useState<Project | null>(null)

  useEffect(() => {
    if (open) setAdding(false)
    else setMenu(null)
  }, [open])

  const projects = useQuery({ ...projectsQuery, enabled: open })

  const remove = useMutation({
    mutationFn: deleteProject,
    onSuccess: (_, path) => {
      queryClient.invalidateQueries({ queryKey: keys.projects })
      if (path === value) onChange('', false)
      setConfirm(null)
    },
  })

  const selected = projects.data?.find((project) => project.path === value)
  const label = value ? (selected?.name ?? directoryName(value)) : 'Work in a Project'

  const select = (path: string, git: boolean) => {
    onChange(path, git)
    setOpen(false)
  }

  return (
    <>
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement={placement}
      trigger={
        <Button
          variant="secondary"
          size="sm"
          className="max-w-[13rem]"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={value ? `Project: ${value}` : 'Work in a Project'}
          title={value || 'Work in a Project'}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <Folder size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
        </Button>
      }
    >
      <div className="w-[300px]">
        {adding ? (
          <DirectoryBrowser
            initialPath={value}
            onBack={() => setAdding(false)}
            onAdded={(path, git) => {
              queryClient.invalidateQueries({ queryKey: keys.projects })
              select(path, git)
            }}
          />
        ) : (
          <>
            <button
              type="button"
              onClick={() => setAdding(true)}
              className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
            >
              <FolderPlus size={13} className="shrink-0 text-primary" />
              Add new project
            </button>
            <div className="my-1 border-t border-border" />
            <MenuRow selected={value === ''} onClick={() => select('', false)}>
              Default directory
            </MenuRow>
            <div className="max-h-[220px] overflow-y-auto">
              {projects.isLoading ? (
                <div className="flex h-7 items-center gap-2 px-2 text-[13px] text-ink-3">
                  <LoaderCircle size={13} className="animate-spin" />
                  Loading…
                </div>
              ) : projects.isError ? (
                <div className="px-2 py-1 text-[13px] text-ink-3">Couldn't read projects.</div>
              ) : projects.data && projects.data.length > 0 ? (
                projects.data.map((project) => (
                  <ProjectRow
                    key={project.path}
                    project={project}
                    selected={project.path === value}
                    onSelect={() => select(project.path, project.git)}
                    onContextMenu={(point) => setMenu({ point, project })}
                  />
                ))
              ) : (
                <div className="px-2 py-1 text-[13px] text-ink-3">No projects yet.</div>
              )}
            </div>
            {projects.data && projects.data.length > 0 ? (
              <p className="px-2.5 pt-1.5 text-[11px] text-ink-3">Right-click a project to remove it.</p>
            ) : null}
          </>
        )}
      </div>
    </Popover>
    {menu ? (
      <ContextMenu point={menu.point} onClose={() => setMenu(null)}>
        <MenuRow
          onClick={() => {
            setConfirm(menu.project)
            setMenu(null)
          }}
        >
          <span className="flex items-center gap-2 text-danger">
            <Trash2 size={13} />
            Remove from projects
          </span>
        </MenuRow>
      </ContextMenu>
    ) : null}
    {confirm ? (
      <Modal
        open
        onClose={() => {
          if (!remove.isPending) setConfirm(null)
        }}
        title="Remove project"
        size="sm"
        footer={
          <div className="flex w-full justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirm(null)} disabled={remove.isPending}>
              Cancel
            </Button>
            <Button variant="danger" onClick={() => remove.mutate(confirm.path)} disabled={remove.isPending}>
              {remove.isPending ? 'Removing…' : 'Remove'}
            </Button>
          </div>
        }
      >
        <p className="text-[13px] text-ink-2">
          Remove <span className="font-medium text-ink">{confirm.name}</span> from your projects? This only
          takes it off the list — the folder and its files stay on disk.
        </p>
        {remove.isError ? (
          <p className="mt-2 text-[12px] text-danger">{(remove.error as Error).message}</p>
        ) : null}
      </Modal>
    ) : null}
    </>
  )
}

// A saved-project row that selects on click and opens a remove menu on
// right-click / press-and-hold. The context-menu trigger's own click handler
// runs first and only swallows the post-long-press tap, so a normal click
// still selects.
function ProjectRow({
  project,
  selected,
  onSelect,
  onContextMenu,
}: {
  project: Project
  selected: boolean
  onSelect: () => void
  onContextMenu: (point: { x: number; y: number }) => void
}) {
  const triggers = useContextMenuTrigger(onContextMenu)
  return (
    <button
      type="button"
      {...triggers}
      onClick={(e) => {
        triggers.onClick(e)
        if (!e.defaultPrevented) onSelect()
      }}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 hover:bg-surface-2 ${
        selected ? 'text-ink' : 'text-ink-2'
      }`}
      title={project.path}
    >
      <Folder size={13} className="shrink-0 text-ink-3" />
      <span className="min-w-0 flex-1 truncate">{project.name}</span>
      {project.git ? (
        <GitBranch size={12} className="shrink-0 text-ink-3" aria-label="git repository" />
      ) : null}
      {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}

// Browses directories on the (remote) server to add a folder as a project.
// Mounts only while the picker is in add mode, so its browse state and listing
// query live and die with the view instead of sitting on the parent picker.
function DirectoryBrowser({
  initialPath,
  onBack,
  onAdded,
}: {
  initialPath: string
  onBack: () => void
  onAdded: (path: string, git: boolean) => void
}) {
  const [browse, setBrowse] = useState(initialPath)
  const [typing, setTyping] = useState(false)
  const [pathInput, setPathInput] = useState('')

  const dirs = useQuery({
    queryKey: keys.filesystemDirs(browse),
    queryFn: () => listFilesystemDirs(browse),
  })
  const add = useMutation({
    mutationFn: addProject,
    onSuccess: (project) => onAdded(project.path, project.git),
  })

  const currentPath = dirs.data?.path ?? browse
  const crumbs = useMemo(() => buildCrumbs(currentPath), [currentPath])

  const goTo = (path: string) => {
    setBrowse(path)
    setTyping(false)
  }

  const submitPath = () => {
    const next = pathInput.trim()
    if (next) goTo(next)
    else setTyping(false)
  }

  return (
    <>
      <div className="flex items-center gap-0.5 px-1 pt-1 pb-1">
        <IconButton
          variant="ghost"
          size="xs"
          aria-label="Back to projects"
          title="Back to projects"
          onClick={onBack}
        >
          <ChevronLeft size={15} />
        </IconButton>
        {typing ? (
          <>
            <input
              autoFocus
              value={pathInput}
              spellCheck={false}
              placeholder="Path to project"
              onChange={(e) => setPathInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  submitPath()
                } else if (e.key === 'Escape') {
                  setTyping(false)
                }
              }}
              className="h-7 min-w-0 flex-1 rounded-[6px] bg-ink/10 px-2 font-mono text-[11px] text-ink outline-none placeholder:text-ink-3 focus:bg-ink/15"
            />
            <IconButton variant="ghost" size="xs" aria-label="Go to path" title="Go to path" onClick={submitPath}>
              <ArrowRight size={14} />
            </IconButton>
          </>
        ) : (
          <>
            <div className="flex min-w-0 flex-1 items-center gap-0.5 overflow-hidden">
              <button
                type="button"
                onClick={() => goTo('/')}
                className="shrink-0 rounded-[6px] px-1 py-0.5 text-[12px] text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                title="Filesystem root"
              >
                /
              </button>
              {crumbs.map((crumb, index) => {
                const isLast = index === crumbs.length - 1
                return (
                  <Fragment key={`${crumb.path}:${crumb.name}`}>
                    <span className="shrink-0 text-[11px] text-ink-3">›</span>
                    {isLast && !crumb.collapsed ? (
                      <span
                        className="min-w-0 flex-1 truncate px-1 py-0.5 text-[12px] font-medium text-ink"
                        title={currentPath}
                      >
                        {crumb.name}
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => goTo(crumb.path)}
                        className="max-w-[7rem] shrink-0 truncate rounded-[6px] px-1 py-0.5 text-[12px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                        title={crumb.collapsed ? 'Show parent folders' : crumb.path}
                      >
                        {crumb.name}
                      </button>
                    )}
                  </Fragment>
                )
              })}
            </div>
            <IconButton
              variant="ghost"
              size="xs"
              aria-label="Type a path"
              title="Type a path"
              onClick={() => {
                setPathInput(currentPath)
                setTyping(true)
              }}
            >
              <Keyboard size={14} />
            </IconButton>
          </>
        )}
      </div>
      <p className="px-2.5 pb-1.5 text-[11px] text-ink-3">
        Open folders to browse, then choose one as your project.
      </p>
      <div className="max-h-[240px] overflow-y-auto">
        {dirs.isLoading ? (
          <div className="flex h-7 items-center gap-2 px-2 text-[13px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" />
            Loading…
          </div>
        ) : dirs.isError ? (
          <div className="px-2 py-1 text-[13px] text-ink-3">Couldn't read this folder.</div>
        ) : dirs.data && dirs.data.dirs.length > 0 ? (
          dirs.data.dirs.map((dir) => (
            <button
              key={dir.path}
              type="button"
              onClick={() => goTo(dir.path)}
              className="group flex h-8 w-full items-center gap-2 rounded-[8px] px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
              title={dir.path}
            >
              <Folder size={14} className="shrink-0 text-primary/70" />
              <span className="min-w-0 flex-1 truncate">{dir.name}</span>
              {dir.git ? (
                <GitBranch size={12} className="shrink-0 text-ink-3" aria-label="git repository" />
              ) : null}
              <ChevronRight size={14} className="shrink-0 text-ink-3 opacity-0 transition-opacity duration-150 group-hover:opacity-100" />
            </button>
          ))
        ) : (
          <div className="px-2 py-1 text-[13px] text-ink-3">No subfolders here.</div>
        )}
      </div>
      <div className="mt-1 border-t border-border pt-1">
        <button
          type="button"
          disabled={!dirs.data || add.isPending}
          onClick={() => dirs.data && add.mutate(dirs.data.path)}
          className="flex h-8 w-full items-center gap-2 rounded-[8px] bg-primary-soft px-2.5 text-left text-[13px] font-medium text-primary transition-colors duration-150 hover:bg-primary-soft/80 disabled:cursor-default disabled:opacity-50"
        >
          {add.isPending ? (
            <LoaderCircle size={14} className="shrink-0 animate-spin" />
          ) : (
            <Check size={14} className="shrink-0" />
          )}
          <span className="min-w-0 flex-1 truncate">Choose “{directoryName(currentPath)}”</span>
        </button>
      </div>
      {add.isError ? (
        <div className="px-2 pt-1 text-[12px] text-danger">{(add.error as Error).message}</div>
      ) : null}
    </>
  )
}
