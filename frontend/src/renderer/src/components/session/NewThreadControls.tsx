import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, CornerLeftUp, Folder, GitBranch, LoaderCircle } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { agentLabel } from '@/lib/agentLabel'
import { listWorkspaceDirs } from '@/lib/api/sessions'
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
// a provider section sits above the model list.
export function ModelSelect({
  value,
  suggestions,
  loading,
  disabled,
  onChange,
  providers,
  provider,
  onProviderChange,
}: {
  value: string
  suggestions: ModelSuggestion[]
  loading?: boolean
  disabled?: boolean
  onChange: (model: string) => void
  providers?: { value: string; label: string }[]
  provider?: string
  onProviderChange?: (provider: string) => void
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
          className="max-w-[11rem]"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={`Model: ${value === '' ? 'default' : value}`}
          title={`Model: ${value === '' ? 'default' : value}`}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="truncate">{label}</span>
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
        {/* Shorter list when the provider section stacks above it, so the
            popover stays inside the window above a centered composer. */}
        <div
          className={`${providers && providers.length > 1 ? 'max-h-[170px]' : 'max-h-[240px]'} overflow-y-auto`}
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
      </div>
    </Popover>
  )
}

function joinPath(base: string, name: string): string {
  return base ? `${base}/${name}` : name
}

function parentPath(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

// Browses the workspace (confined server-side) to choose the session working
// directory. `value` is a workspace-relative path; '' is the workspace root.
export function DirectoryPicker({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  // git reports whether the chosen path is a git repository root, so callers can
  // offer worktree-only options for it.
  onChange: (path: string, git: boolean) => void
}) {
  const [open, setOpen] = useState(false)
  // The path currently being browsed; it starts at the selection each time the
  // picker opens so reopening lands where the user left off.
  const [browse, setBrowse] = useState(value)
  useEffect(() => {
    if (open) setBrowse(value)
  }, [open, value])

  const query = useQuery({
    queryKey: keys.workspaceDirs(browse),
    queryFn: () => listWorkspaceDirs(browse),
    enabled: open,
  })

  const label = value === '' ? 'workspace' : (value.split('/').at(-1) ?? value)
  const browseLabel = browse === '' ? 'workspace' : browse
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
          className="max-w-[12rem]"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={`Directory: ${value === '' ? 'workspace root' : value}`}
          title={value === '' ? 'Workspace root' : value}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <Folder size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
        </Button>
      }
    >
      <div className="flex items-center gap-1 px-2 pt-1 pb-1.5">
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-ink-3" title={browseLabel}>
          {browseLabel}
        </span>
        {browse !== '' ? (
          <IconButton
            variant="ghost"
            size="xs"
            aria-label="Parent directory"
            title="Parent directory"
            onClick={() => setBrowse(parentPath(browse))}
          >
            <CornerLeftUp size={14} />
          </IconButton>
        ) : null}
      </div>
      <div className="max-h-[220px] overflow-y-auto">
        {query.isLoading ? (
          <div className="flex h-7 items-center gap-2 px-2 text-[13px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" />
            Loading…
          </div>
        ) : query.isError ? (
          <div className="px-2 py-1 text-[13px] text-ink-3">Couldn't read this folder.</div>
        ) : query.data && query.data.dirs.length > 0 ? (
          query.data.dirs.map((dir) => (
            <button
              key={dir.name}
              type="button"
              onClick={() => setBrowse(joinPath(browse, dir.name))}
              className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
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
          onClick={() => select(browse, query.data?.git ?? false)}
          className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
        >
          <Check size={13} className="shrink-0 text-primary" />
          Use this folder
        </button>
      </div>
    </Popover>
  )
}
