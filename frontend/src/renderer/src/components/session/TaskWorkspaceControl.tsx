import { GitBranch } from 'lucide-react'
import type { RepoRevision } from '@/lib/api/types'

export function TaskWorkspaceControl({ worktree, revision, directory, disabled, onChange }: {
  worktree: boolean
  revision?: RepoRevision
  directory: string
  disabled: boolean
  onChange: (worktree: boolean) => void
}) {
  const source = revision?.branch || revision?.head
    ? [revision.branch || 'HEAD', revision.head?.slice(0, 7)].filter(Boolean).join(' · ')
    : 'source unknown'
  return (
    <div className="flex min-w-0 items-center gap-2 px-2 text-[12px] text-ink-2">
      <GitBranch size={14} className="shrink-0 text-ink-3" aria-hidden />
      <select
        aria-label="Task workspace"
        value={worktree ? 'worktree' : 'local'}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value === 'worktree')}
        className="h-8 min-w-0 cursor-pointer rounded-lg bg-transparent text-[13px] disabled:cursor-default disabled:opacity-50"
      >
        <option value="worktree">New worktree</option>
        <option value="local">Local checkout</option>
      </select>
      <span className="min-w-0 truncate text-ink-3" title={`${directory}\n${revision?.head || 'Current HEAD unavailable'}${worktree ? '\nStarts from committed HEAD; source edits stay in the local checkout.' : ''}`}>
        {worktree ? 'From ' : 'On '}{source}
      </span>
    </div>
  )
}
