import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ChevronDown, CornerLeftUp, Folder, FolderPlus, GitBranch, LoaderCircle } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { REASONING_EFFORT_OPTIONS } from '@/components/loops/ReasoningEffortSelect'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { agentLabel } from '@/lib/agentLabel'
import { addProject, listFilesystemDirs, projectsQuery } from '@/lib/api/sessions'
import {
  filterModelSuggestions,
  modelSuggestionLabel,
  type ModelSuggestion,
} from '@/lib/models'
import { keys } from '@/lib/query/keys'

// Selects the runtime backing a new thread: Native (the default Jaz session) or
// one of the configured ACP agents. `value` is 'native' or an agent name.
export function RuntimeSelect({
  value,
  agents,
  disabled,
  onChange,
}: {
  value: string
  agents: string[]
  disabled?: boolean
  onChange: (runtime: string) => void
}) {
  const [open, setOpen] = useState(false)
  const label = value === 'native' ? 'Native' : agentLabel(value)
  const select = (runtime: string) => {
    onChange(runtime)
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      trigger={
        <Button
          variant="secondary"
          size="sm"
          className="max-w-[12rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={`Runtime: ${label}`}
          title={`Runtime: ${label}`}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="truncate">{label}</span>
          <ChevronDown size={13} className="shrink-0" />
        </Button>
      }
    >
      <MenuRow selected={value === 'native'} onClick={() => select('native')}>
        Native
      </MenuRow>
      {agents.map((agent) => (
        <MenuRow key={agent} selected={value === agent} onClick={() => select(agent)}>
          {agentLabel(agent)}
        </MenuRow>
      ))}
    </Popover>
  )
}

// Picks the model for a new thread: curated suggestions for the chosen
// runtime/provider plus free-text entry for anything else. For native threads
// a provider section sits above the model list; a reasoning-effort section
// sits below it when the caller wires one up. The trigger shows the effective
// effort as a tinted suffix ("GPT-5.4 Mini xhigh").
export function ModelSelect({
  value,
  suggestions,
  loading,
  disabled,
  onChange,
  providers,
  provider,
  onProviderChange,
  effort,
  onEffortChange,
}: {
  value: string
  suggestions: ModelSuggestion[]
  loading?: boolean
  disabled?: boolean
  onChange: (model: string) => void
  providers?: { value: string; label: string }[]
  provider?: string
  onProviderChange?: (provider: string) => void
  // '' inherits the Settings > Agents default for the chosen runtime/provider.
  effort?: string
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
              {REASONING_EFFORT_OPTIONS.map((option) => (
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

export function ProjectPicker({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (path: string, git: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [browse, setBrowse] = useState(value)

  useEffect(() => {
    if (open) {
      setAdding(false)
      setBrowse(value)
    }
  }, [open, value])

  const projects = useQuery({ ...projectsQuery, enabled: open })
  const dirs = useQuery({
    queryKey: keys.filesystemDirs(browse),
    queryFn: () => listFilesystemDirs(browse),
    enabled: open && adding,
  })
  const add = useMutation({
    mutationFn: addProject,
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: keys.projects })
      onChange(project.path, project.git)
      setOpen(false)
      setAdding(false)
    },
  })

  const selected = projects.data?.find((project) => project.path === value)
  const label = value ? (selected?.name ?? directoryName(value)) : 'Work in a Project'
  const currentPath = dirs.data?.path ?? browse

  const select = (path: string, git: boolean) => {
    onChange(path, git)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
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
        {!adding ? (
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
                onClick={() => {
                  setBrowse(value)
                  setAdding(true)
                }}
                className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
              >
                <FolderPlus size={13} className="shrink-0 text-primary" />
                Add new project
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="flex items-center gap-1 px-2 pt-1 pb-1.5">
              <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-ink-3" title={currentPath}>
                {currentPath || 'home'}
              </span>
              {dirs.data?.parent ? (
                <IconButton
                  variant="ghost"
                  size="xs"
                  aria-label="Parent directory"
                  title="Parent directory"
                  onClick={() => setBrowse(dirs.data?.parent ?? '')}
                >
                  <CornerLeftUp size={14} />
                </IconButton>
              ) : null}
            </div>
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
                  <button
                    key={dir.path}
                    type="button"
                    onClick={() => setBrowse(dir.path)}
                    className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                    title={dir.path}
                  >
                    <Folder size={13} className="shrink-0 text-ink-3" />
                    <span className="min-w-0 flex-1 truncate">{dir.name}</span>
                    {dir.git ? (
                      <GitBranch size={12} className="shrink-0 text-ink-3" aria-label="git repository" />
                    ) : null}
                  </button>
                ))
              ) : (
                <div className="px-2 py-1 text-[13px] text-ink-3">No subfolders.</div>
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
                Use this folder
              </button>
              <button
                type="button"
                onClick={() => setAdding(false)}
                className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
              >
                <CornerLeftUp size={13} className="shrink-0 text-ink-3" />
                Back to projects
              </button>
            </div>
            {add.isError ? (
              <div className="px-2 pt-1 text-[12px] text-danger">{(add.error as Error).message}</div>
            ) : null}
          </>
        )}
      </div>
    </Popover>
  )
}
