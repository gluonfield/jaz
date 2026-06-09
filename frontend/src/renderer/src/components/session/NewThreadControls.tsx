import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, CornerLeftUp, Folder, LoaderCircle } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { agentLabel } from '@/lib/agentLabel'
import { listWorkspaceDirs } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'

export const TRIGGER_CLASS =
  'inline-flex h-8 max-w-[12rem] items-center gap-1.5 rounded-control border border-border bg-bg px-2 text-[12px] font-medium text-ink-2 transition-colors duration-150 hover:border-primary hover:text-primary disabled:cursor-default disabled:opacity-50'

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
        <motion.button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label={`Runtime: ${label}`}
          title={`Runtime: ${label}`}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
          whileTap={{ scale: 0.96 }}
          className={TRIGGER_CLASS}
        >
          <span className="truncate">{label}</span>
          <ChevronDown size={13} className="shrink-0" />
        </motion.button>
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
  onChange: (path: string) => void
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
  const select = (path: string) => {
    onChange(path)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      trigger={
        <motion.button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={`Directory: ${value === '' ? 'workspace root' : value}`}
          title={value === '' ? 'Workspace root' : value}
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
          whileTap={{ scale: 0.96 }}
          className={TRIGGER_CLASS}
        >
          <Folder size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
        </motion.button>
      }
    >
      <div className="flex items-center gap-1 px-2 pt-1 pb-1.5">
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-ink-3" title={browseLabel}>
          {browseLabel}
        </span>
        {browse !== '' ? (
          <button
            type="button"
            aria-label="Parent directory"
            title="Parent directory"
            onClick={() => setBrowse(parentPath(browse))}
            className="grid size-6 place-items-center rounded-control text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
          >
            <CornerLeftUp size={14} />
          </button>
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
              key={dir}
              type="button"
              onClick={() => setBrowse(joinPath(browse, dir))}
              className="flex h-7 w-full items-center gap-2 rounded-control px-2 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              <Folder size={13} className="shrink-0 text-ink-3" />
              <span className="min-w-0 flex-1 truncate">{dir}</span>
            </button>
          ))
        ) : (
          <div className="px-2 py-1 text-[13px] text-ink-3">No subfolders.</div>
        )}
      </div>
      <div className="mt-1 border-t border-border pt-1">
        <button
          type="button"
          onClick={() => select(browse)}
          className="flex h-7 w-full items-center gap-2 rounded-control px-2 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
        >
          <Check size={13} className="shrink-0 text-primary" />
          Use this folder
        </button>
      </div>
    </Popover>
  )
}
