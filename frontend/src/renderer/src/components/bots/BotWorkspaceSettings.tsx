import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/Button'
import { Checkbox } from '@/components/ui/Checkbox'
import { Modal } from '@/components/ui/Modal'
import { projectsQuery } from '@/lib/api/sessions'
import type { Bot } from '@/lib/api/types'
import { useUpdateBot } from './useUpdateBot'

export function BotWorkspacePicker({ value, onChange }: { value: string[]; onChange: (paths: string[]) => void }) {
  const projects = useQuery(projectsQuery)
  const choices = [
    ...(projects.data ?? []),
    ...value.filter((path) => !projects.data?.some((project) => project.path === path)).map((path) => ({ path, name: path })),
  ]
  return (
    <div className="mt-4">
      <p className="text-[13px] font-medium text-ink">Project knowledge and tools</p>
      <p className="mt-1 text-[12px] text-ink-3">Choose folders on the backend. The bot and its workers read project instructions there; private bot memory stays in its own home.</p>
      {choices.length > 0 && value.length === 0 ? <p className="mt-2 text-[12px] text-ink-3">No project selected. This bot will start with its own instructions and memory only.</p> : null}
      {projects.isPending ? <p className="mt-3 text-[13px] text-ink-3">Loading projects…</p> : null}
      {projects.isError ? <p className="mt-3 text-[13px] text-danger">Couldn't load projects: {projects.error.message}</p> : null}
      {choices.length === 0 && projects.isSuccess ? <p className="mt-3 text-[13px] text-ink-3">No projects registered. The bot will use its private home.</p> : null}
      <div className="mt-2 flex max-h-52 flex-col gap-px overflow-y-auto">
        {choices.map((project) => (
          <label key={project.path} className="flex min-h-9 cursor-pointer items-center gap-2.5 rounded-lg px-2 hover:bg-list-hover">
            <Checkbox checked={value.includes(project.path)} aria-label={project.name} onChange={(checked) => onChange(checked ? [...value, project.path] : value.filter((path) => path !== project.path))} />
            <span className="min-w-0 flex-1 truncate text-[13px] text-ink">{project.name}</span>
            <span className="max-w-40 truncate text-[11px] text-ink-3" title={project.path}>{project.path}</span>
          </label>
        ))}
      </div>
    </div>
  )
}

export function BotWorkspaceSettings({ bot }: { bot: Bot }) {
  const [open, setOpen] = useState(false)
  const [paths, setPaths] = useState<string[]>([])
  const update = useUpdateBot(bot.id)
  return (
    <>
      <button type="button" className="max-w-full truncate text-[11px] text-ink-3 underline decoration-dotted underline-offset-4 hover:text-ink" title={bot.workspaces?.join('\n')} onClick={() => { setPaths(bot.workspaces ?? []); setOpen(true) }}>
        {bot.workspaces?.length ? `Projects: ${bot.workspaces.map((path) => path.split('/').filter(Boolean).at(-1)).join(', ')}` : 'Connect projects'}
      </button>
      <Modal open={open} onClose={() => setOpen(false)} title="Bot projects" size="sm" footer={
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button variant="primary" disabled={update.isPending} onClick={() => update.mutate({ workspaces: paths }, { onSuccess: () => setOpen(false) })}>Save</Button>
        </div>
      }>
        <BotWorkspacePicker value={paths} onChange={setPaths} />
      </Modal>
    </>
  )
}
