import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Folder,
  FolderPlus,
  GitBranch,
  LoaderCircle,
  Trash2,
  X,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { CreateProjectDialog } from '@/components/projects/CreateProjectDialog'
import { Modal } from '@/components/ui/Modal'
import { ContextMenu, MenuRow, Popover } from '@/components/ui/Popover'
import { deleteProject, projectsQuery, type Project } from '@/lib/api/sessions'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { keys } from '@/lib/query/keys'

function directoryName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts.at(-1) ?? path
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
  const triggerRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [menu, setMenu] = useState<{ point: { x: number; y: number }; project: Project } | null>(null)
  const [confirm, setConfirm] = useState<Project | null>(null)

  useEffect(() => {
    if (!open) setMenu(null)
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
  const label = value ? (selected?.name ?? directoryName(value)) : 'Choose a project'

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
        <div className={`inline-flex h-7 max-w-[13rem] items-center rounded-full text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] duration-150 ${disabled ? 'opacity-50' : 'hover:bg-surface-2 hover:text-ink active:scale-[0.96]'}`}>
          {value ? (
            <button
              type="button"
              aria-label="No project"
              title="No project"
              disabled={disabled}
              onClick={() => select('', false)}
              className="group/clear grid h-full w-7 shrink-0 cursor-pointer place-items-center rounded-full disabled:cursor-default"
            >
              <Folder size={13} className="col-start-1 row-start-1 group-enabled/clear:group-hover/clear:invisible group-focus-visible/clear:invisible [@media(hover:none)]:group-enabled/clear:invisible" />
              <X size={13} className="invisible col-start-1 row-start-1 group-enabled/clear:group-hover/clear:visible group-focus-visible/clear:visible [@media(hover:none)]:group-enabled/clear:visible" />
            </button>
          ) : null}
          <button
            ref={triggerRef}
            type="button"
            className={`flex h-full min-w-0 flex-1 cursor-pointer items-center justify-center gap-1.5 rounded-full px-2.5 disabled:cursor-default ${value ? 'pl-0' : ''}`}
            aria-haspopup="dialog"
            aria-expanded={open}
            aria-label={value ? `Project: ${value}` : label}
            title={value || label}
            disabled={disabled}
            onClick={() => setOpen((v) => !v)}
          >
            {value ? null : <Folder size={13} className="shrink-0" />}
            <span className="truncate">{label}</span>
          </button>
        </div>
      }
    >
      <div className="w-[300px]">
        <button
          type="button"
          onClick={() => {
            triggerRef.current?.focus()
            setOpen(false)
            setAdding(true)
          }}
          className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] font-medium text-ink transition-colors duration-150 hover:bg-primary-soft"
        >
          <FolderPlus size={13} className="shrink-0 text-primary" />
          New project
        </button>
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
        <div className="my-1 border-t border-border" />
        <MenuRow selected={value === ''} onClick={() => select('', false)}>
          <span className="flex items-center gap-2">
            <X size={13} className="shrink-0" />
            No project
          </span>
        </MenuRow>
      </div>
    </Popover>
    {adding ? (
      <CreateProjectDialog
        initialPath={value}
        onClose={() => setAdding(false)}
        onCreated={(project) => {
          setAdding(false)
          select(project.path, project.git)
        }}
      />
    ) : null}
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
