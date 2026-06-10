import { useQuery, useQueryClient } from '@tanstack/react-query'
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
import { useToast } from '@/components/ui/toast'
import {
  commitSessionRepo,
  mergeSessionRepo,
  pushSessionRepoBranch,
  sessionRepoQuery,
} from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

type Busy = 'pr' | 'commit' | 'push' | 'merge' | null

// Titlebar repo capsule: shows the working directory's branch and unfolds
// into repo actions. Forge link-outs (create PR, open repo) need a parseable
// web remote; the local actions (commit, handoff) work on any git cwd.
export function RepoActions({ session }: { session: Session }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState<Busy>(null)
  const toast = useToast()
  const queryClient = useQueryClient()
  const cwd = session.runtime_ref?.cwd
  const repo = useQuery({ ...sessionRepoQuery(session.id), enabled: Boolean(cwd) })
  const info = repo.data
  if (!cwd || !info?.git) return null

  const web = info.web_url
  const branch = info.branch ?? ''
  const onDefaultBranch = Boolean(branch) && branch === info.default_branch
  // Keep slashes literal so feature/x branches map onto forge URLs.
  const branchPath = branch.split('/').map(encodeURIComponent).join('/')
  const defaultPath = (info.default_branch ?? '').split('/').map(encodeURIComponent).join('/')
  const compareUrl = info.default_branch
    ? `${web}/compare/${defaultPath}...${branchPath}?expand=1`
    : `${web}/pull/new/${branchPath}`
  // Cases no automation can fix stay disabled with an explanation; dirty work
  // and a missing upstream are handled by createPR committing/pushing first.
  const prHint = !branch
    ? 'Detached HEAD — check out a branch first'
    : onDefaultBranch
      ? 'Already on the default branch'
      : info.no_commits && !info.dirty
        ? 'No changes on this branch yet'
        : info.dirty
          ? 'Commits, pushes, then opens GitHub'
          : !info.has_upstream
            ? 'Pushes the branch, then opens GitHub'
            : undefined
  const prDisabled = !branch || onDefaultBranch || Boolean(info.no_commits && !info.dirty)

  const setRepoData = (next: typeof info) =>
    queryClient.setQueryData(keys.sessionRepo(session.id), next)
  const openUrl = (url: string) => {
    // The main process routes window.open to the system browser.
    window.open(url, '_blank', 'noopener')
    setOpen(false)
  }
  const run = async (kind: Exclude<Busy, null>, fn: () => Promise<void>) => {
    setBusy(kind)
    try {
      await fn()
    } catch (error) {
      toast((error as Error).message, 'danger')
      // The repo may have changed server-side even when the action failed.
      void repo.refetch()
    } finally {
      setBusy(null)
    }
  }

  const createPR = () =>
    run('pr', async () => {
      let state = info
      if (state.dirty) {
        state = await commitSessionRepo(session.id)
        setRepoData(state)
      }
      if (!state.has_upstream) {
        state = await pushSessionRepoBranch(session.id)
        setRepoData(state)
      }
      openUrl(compareUrl)
    })
  const commit = () =>
    run('commit', async () => {
      setRepoData(await commitSessionRepo(session.id))
      toast('Changes committed')
    })
  const push = () =>
    run('push', async () => {
      setRepoData(await pushSessionRepoBranch(session.id))
      toast('Branch pushed')
    })
  const merge = () =>
    run('merge', async () => {
      const result = await mergeSessionRepo(session.id)
      setRepoData(result.info)
      // A moved cwd changes the session row; refetch it.
      if (result.moved) void queryClient.invalidateQueries({ queryKey: keys.sessionMessages(session.id) })
      toast(
        result.moved
          ? `Merged into ${info.main_branch} — the session now works in the main checkout`
          : `Merged into ${info.main_branch} — the agent keeps working in the worktree`,
      )
      setOpen(false)
    })

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={
        <button
          type="button"
          title={`${web ? `${info.owner}/${info.repo}` : cwd} · ${branch || 'detached'}`}
          onClick={() => {
            // Reads can go stale mid-session (agent pushes, switches branch);
            // refresh on open so the menu reflects the repo right now.
            if (!open) void repo.refetch()
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
          disabled={prDisabled || busy !== null}
          hint={prHint}
          onClick={createPR}
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
          onClick={commit}
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
          onClick={push}
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
          onClick={merge}
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

function ActionRow({
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
