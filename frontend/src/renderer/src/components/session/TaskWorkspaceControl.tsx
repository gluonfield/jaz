import { Checkbox } from '@/components/ui/Checkbox'
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
    <label
      className={`flex h-8 shrink-0 items-center gap-2 px-2 text-[13px] text-ink-2 ${disabled ? 'opacity-50' : 'cursor-pointer'}`}
      title={`Run in a separate Git worktree from ${source}. Uncommitted changes stay in the current checkout.\n${directory}`}
    >
      <Checkbox checked={worktree} onChange={onChange} disabled={disabled} aria-label="Run on a git worktree" />
      Worktree
    </label>
  )
}
