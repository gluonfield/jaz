import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, CornerLeftUp, Folder, GitBranch, LoaderCircle } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { agentLabel } from '@/lib/agentLabel'
import { listWorkspaceDirs } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'

// A floating menu anchored above its trigger, dismissed on outside-click/Escape.
// Trigger and menu share one wrapper so clicking the trigger doesn't self-close.
export function Popover({
  open,
  onClose,
  trigger,
  children,
}: {
  open: boolean
  onClose: () => void
  trigger: ReactNode
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  const reducedMotion = useReducedMotion()

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  return (
    <div ref={ref} className="relative">
      {trigger}
      <AnimatePresence>
        {open ? (
          <motion.div
            initial={{ opacity: 0, y: reducedMotion ? 0 : 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: reducedMotion ? 0 : 6 }}
            transition={{ duration: 0.15, ease: 'easeOut' }}
            className="absolute bottom-full left-0 z-20 mb-1.5 min-w-[176px] rounded-[10px] bg-surface p-1 shadow-xl ring-1 ring-border"
          >
            {children}
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

export function MenuRow({
  selected,
  onClick,
  children,
}: {
  selected?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex h-7 w-full items-center gap-2 rounded-control px-2 text-left text-[13px] transition-colors duration-150 hover:bg-surface-2 ${
        selected ? 'text-ink' : 'text-ink-2'
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {selected ? <Check size={13} className="shrink-0 text-primary" /> : null}
    </button>
  )
}

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
          size="md"
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

function joinPath(base: string, name: string): string {
  return base ? `${base}/${name}` : name
}

function parentPath(path: string): string {
  const idx = path.lastIndexOf('/')
  return idx === -1 ? '' : path.slice(0, idx)
}

// Browses the workspace (confined server-side) to choose where an ACP coding
// agent runs. `value` is a workspace-relative path; '' is the workspace root.
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
          size="md"
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
              className="flex h-7 w-full items-center gap-2 rounded-control px-2 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
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
          className="flex h-7 w-full items-center gap-2 rounded-control px-2 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
        >
          <Check size={13} className="shrink-0 text-primary" />
          Use this folder
        </button>
      </div>
    </Popover>
  )
}
