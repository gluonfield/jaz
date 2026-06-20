import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowRight,
  Check,
  ChevronDown,
  ChevronLeft,
  Folder,
  FolderPlus,
  GitBranch,
  Keyboard,
  LoaderCircle,
  Plus,
} from 'lucide-react'
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { AgentLogo, hasAgentLogo } from '@/components/acp/AgentLogo'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { agentLabel } from '@/lib/agentLabel'
import { addProject, listFilesystemDirs, projectsQuery } from '@/lib/api/sessions'
import type { ReasoningEffortOption } from '@/lib/api/types'
import {
  filterModelSuggestions,
  modelSuggestionLabel,
  type ModelSuggestion,
} from '@/lib/models'
import { keys } from '@/lib/query/keys'
import { REASONING_EFFORT_OPTIONS } from '@/lib/reasoningEfforts'

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
// agent/provider plus free-text entry for anything else. A provider section
// appears for provider-backed ACP agents.
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
  const effortValue = effort ?? ''
  const description = `Model: ${value === '' ? 'default' : value}${
    effortValue ? `, reasoning effort: ${effortValue}` : ''
  }`
  const select = (model: string) => {
    onChange(model)
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
          className="max-w-[13rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={description}
          title={description}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="truncate">{label}</span>
          {effortValue ? <span className="shrink-0 text-primary">{effortValue}</span> : null}
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      <div className="w-[260px]">
        {providers && providers.length > 1 && onProviderChange ? (
          <>
            <p className="px-2 pt-1 pb-0.5 text-[11px] text-ink-3">Provider</p>
            {providers.map((p) => (
              <MenuRow key={p.value} selected={p.value === provider} onClick={() => onProviderChange(p.value)}>
                {p.label}
              </MenuRow>
            ))}
            <div className="my-1 border-t border-border" />
          </>
        ) : null}
        <div className="px-1 pt-1 pb-1.5">
          <input
            ref={inputRef}
            value={query}
            placeholder="Search or type a model id…"
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && typed !== '') {
                e.preventDefault()
                select(typed)
              }
            }}
            className="h-7 w-full rounded-full bg-ink/10 px-2.5 text-[12px] text-ink outline-none placeholder:text-ink-3 focus:bg-ink/15"
          />
        </div>
        {/* Shorter list when the provider/effort sections stack around it, so
            the popover stays inside the window above a centered composer. */}
        <div
          className={`${
            providers && providers.length > 1
              ? onEffortChange
                ? 'max-h-[120px]'
                : 'max-h-[170px]'
              : onEffortChange
                ? 'max-h-[200px]'
                : 'max-h-[240px]'
          } overflow-y-auto`}
        >
          {typedIsNew ? (
            <MenuRow selected={typed === value} onClick={() => select(typed)}>
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
              <button
                key={s.value}
                type="button"
                onClick={() => select(s.value)}
                className={`flex w-full items-start gap-2 rounded-[8px] px-2 py-1 text-left transition-colors duration-150 hover:bg-surface-2 ${
                  s.value === value ? 'text-ink' : 'text-ink-2'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px]">{s.label}</span>
                  {s.description ? (
                    <span className="mt-0.5 block truncate text-[11px] text-ink-3">{s.description}</span>
                  ) : null}
                </span>
                {s.value === value ? <Check size={13} className="mt-1 shrink-0 text-primary" /> : null}
              </button>
            ))
          ) : !typedIsNew ? (
            <div className="px-2 py-1 text-[13px] text-ink-3">No matching models.</div>
          ) : null}
        </div>
        {onEffortChange ? (
          <>
            <div className="my-1 border-t border-border" />
            <p className="px-2 pt-1 pb-0.5 text-[11px] text-ink-3">Reasoning effort</p>
            <div className="flex flex-wrap gap-1 px-1.5 pb-1">
              {effortOptions.map((option) => (
                <button
                  key={option.value || 'default'}
                  type="button"
                  // Stays open: effort is a refinement, not the menu's main action.
                  onClick={() => onEffortChange(option.value)}
                  className={`h-6 rounded-full px-2 text-[12px] transition-colors duration-150 ${
                    effortValue === option.value
                      ? 'bg-primary-soft text-primary-strong'
                      : 'text-ink-2 hover:bg-surface-2 hover:text-ink'
                  }`}
                >
                  {option.label}
                </button>
              ))}
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

  useEffect(() => {
    if (open) setAdding(false)
  }, [open])

  const projects = useQuery({ ...projectsQuery, enabled: open })

  const selected = projects.data?.find((project) => project.path === value)
  const label = value ? (selected?.name ?? directoryName(value)) : 'Work in a Project'

  const select = (path: string, git: boolean) => {
    onChange(path, git)
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
      <div className="w-[270px]">
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
            <MenuRow selected={value === ''} onClick={() => select('', false)}>
              Default directory
            </MenuRow>
            <div className="my-1 border-t border-border" />
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
                  <button
                    key={project.path}
                    type="button"
                    onClick={() => select(project.path, project.git)}
                    className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 hover:bg-surface-2 ${
                      project.path === value ? 'text-ink' : 'text-ink-2'
                    }`}
                    title={project.path}
                  >
                    <Folder size={13} className="shrink-0 text-ink-3" />
                    <span className="min-w-0 flex-1 truncate">{project.name}</span>
                    {project.git ? (
                      <GitBranch size={12} className="shrink-0 text-ink-3" aria-label="git repository" />
                    ) : null}
                    {project.path === value ? <Check size={13} className="shrink-0 text-primary" /> : null}
                  </button>
                ))
              ) : (
                <div className="px-2 py-1 text-[13px] text-ink-3">No projects yet.</div>
              )}
            </div>
            <div className="mt-1 border-t border-border pt-1">
              <button
                type="button"
                onClick={() => setAdding(true)}
                className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
              >
                <FolderPlus size={13} className="shrink-0 text-primary" />
                Add new project
              </button>
            </div>
          </>
        )}
      </div>
    </Popover>
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
              placeholder="/path/to/project"
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
        Open a folder, or add one directly with <span className="text-ink-2">Add</span>.
      </p>
      <div className="max-h-[220px] overflow-y-auto">
        {dirs.isLoading ? (
          <div className="flex h-7 items-center gap-2 px-2 text-[13px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" />
            Loading…
          </div>
        ) : dirs.isError ? (
          <div className="px-2 py-1 text-[13px] text-ink-3">Couldn't read this folder.</div>
        ) : dirs.data && dirs.data.dirs.length > 0 ? (
          dirs.data.dirs.map((dir) => (
            <div
              key={dir.path}
              className="group flex h-7 items-center rounded-full pr-1 transition-colors duration-150 hover:bg-surface-2"
            >
              <button
                type="button"
                onClick={() => goTo(dir.path)}
                className="flex h-7 min-w-0 flex-1 items-center gap-2 rounded-full pl-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 group-hover:text-ink"
                title={dir.path}
              >
                <Folder size={13} className="shrink-0 text-ink-3" />
                <span className="min-w-0 flex-1 truncate">{dir.name}</span>
                {dir.git ? (
                  <GitBranch size={12} className="shrink-0 text-ink-3" aria-label="git repository" />
                ) : null}
              </button>
              <button
                type="button"
                disabled={add.isPending}
                onClick={() => add.mutate(dir.path)}
                aria-label={`Add ${dir.name}`}
                className="flex h-6 shrink-0 items-center gap-1 rounded-full px-2 text-[12px] text-ink-3 transition-colors duration-150 hover:bg-primary-soft hover:text-primary disabled:cursor-default disabled:opacity-50"
              >
                <Plus size={12} className="shrink-0" />
                Add
              </button>
            </div>
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
          className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft disabled:cursor-default disabled:opacity-50"
        >
          {add.isPending ? (
            <LoaderCircle size={13} className="shrink-0 animate-spin text-primary" />
          ) : (
            <Check size={13} className="shrink-0 text-primary" />
          )}
          <span className="min-w-0 flex-1 truncate">Add “{directoryName(currentPath)}” as a project</span>
        </button>
      </div>
      {add.isError ? (
        <div className="px-2 pt-1 text-[12px] text-danger">{(add.error as Error).message}</div>
      ) : null}
    </>
  )
}
