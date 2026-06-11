import {
  ArrowLeftRight,
  ArrowUpFromLine,
  Check,
  ExternalLink,
  GitBranch,
  GitPullRequest,
  GitPullRequestArrow,
  LoaderCircle,
  type LucideIcon,
} from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import type { Session } from '@/lib/api/types'
import { useRepoActions } from './useRepoActions'

// Titlebar repo capsule: shows the working directory's branch and unfolds
// into repo actions. Forge link-outs (create PR, open repo) need a parseable
// web remote; the local actions (commit, handoff) work on any git cwd.
export function RepoActions({ session }: { session: Session }) {
  const [open, setOpen] = useState(false)
  const repo = useRepoActions(session)
  const { info, busy, web, branch, branchPath } = repo
  if (!repo.cwd || !info?.git) return null

  const openUrl = (url: string) => {
    repo.openUrl(url)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={
        <button
          type="button"
          title={`${web ? `${info.owner}/${info.repo}` : repo.cwd} · ${branch || 'detached'}`}
          onClick={() => {
            // Reads can go stale mid-session (agent pushes, switches branch);
            // refresh on open so the menu reflects the repo right now.
            if (!open) repo.refetch()
            setOpen((value) => !value)
          }}
          className="flex h-7 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <GitBranch size={13} className="shrink-0" />
          <span className="max-w-[180px] truncate font-mono text-[11px]">{branch || 'detached'}</span>
        </button>
      }
    >
      {web ? (
        <div className="truncate px-2.5 pt-1 pb-1.5 text-[11px] text-ink-3">
          {info.owner}/{info.repo}
        </div>
      ) : null}
      {web ? (
        <ActionRow
          icon={busy === 'pr' ? LoaderCircle : GitPullRequestArrow}
          spin={busy === 'pr'}
          disabled={repo.prDisabled || busy !== null}
          hint={repo.prHint}
          onClick={() => void repo.createPR().then((ok) => ok && setOpen(false))}
        >
          {busy === 'pr' ? 'Creating pull request…' : 'Create pull request'}
        </ActionRow>
      ) : null}
      {info.dirty ? (
        <ActionRow
          icon={busy === 'commit' ? LoaderCircle : Check}
          spin={busy === 'commit'}
          disabled={busy !== null}
          hint="Commits everything, message from the session title"
          onClick={() => void repo.commit()}
        >
          {busy === 'commit' ? 'Committing…' : 'Commit changes'}
        </ActionRow>
      ) : null}
      {/* One step at a time: commit first, then push what was committed. */}
      {!info.dirty && info.needs_push ? (
        <ActionRow
          icon={busy === 'push' ? LoaderCircle : ArrowUpFromLine}
          spin={busy === 'push'}
          disabled={busy !== null}
          onClick={() => void repo.push()}
        >
          {busy === 'push' ? 'Pushing…' : 'Push branch'}
        </ActionRow>
      ) : null}
      {info.is_worktree && info.main_branch && (info.dirty || !info.no_commits) ? (
        <ActionRow
          icon={busy === 'merge' ? LoaderCircle : ArrowLeftRight}
          spin={busy === 'merge'}
          disabled={busy !== null}
          hint={`Commits this session's work and merges its branch into ${info.main_branch}`}
          onClick={() => void repo.merge().then((ok) => ok && setOpen(false))}
        >
          {busy === 'merge' ? 'Merging…' : `Merge into ${info.main_branch}`}
        </ActionRow>
      ) : null}
      {web ? (
        <>
          <ActionRow icon={GitPullRequest} disabled={busy !== null} onClick={() => openUrl(`${web}/pulls`)}>
            Pull requests
          </ActionRow>
          {branch ? (
            <ActionRow icon={GitBranch} disabled={busy !== null} onClick={() => openUrl(`${web}/tree/${branchPath}`)}>
              View branch
            </ActionRow>
          ) : null}
          <ActionRow icon={ExternalLink} disabled={busy !== null} onClick={() => openUrl(web)}>
            Open repository
          </ActionRow>
        </>
      ) : (
        <div className="max-w-[240px] px-2.5 py-1.5 text-[12px] leading-snug text-ink-3">
          <span className="block">No GitHub remote configured, so there are no link-out actions.</span>
          {info.remote_url ? (
            <span className="mt-1 block truncate font-mono text-[11px]">{info.remote_url}</span>
          ) : null}
        </div>
      )}
    </Popover>
  )
}

export function ActionRow({
  icon: Icon,
  onClick,
  disabled,
  hint,
  spin,
  children,
}: {
  icon: LucideIcon
  onClick: () => void
  disabled?: boolean
  hint?: string
  spin?: boolean
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={hint}
      className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 enabled:cursor-pointer enabled:hover:bg-surface-2 enabled:hover:text-ink disabled:opacity-50"
    >
      <Icon size={13} className={`shrink-0 text-ink-3 ${spin ? 'animate-spin' : ''}`} />
      <span className="min-w-0 flex-1 truncate">{children}</span>
    </button>
  )
}
