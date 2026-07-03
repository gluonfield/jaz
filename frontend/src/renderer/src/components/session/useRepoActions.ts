import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  commitSessionRepo,
  mergeFromMainSessionRepo,
  mergeSessionRepo,
  pushSessionRepoBranch,
  restoreSessionWorktree,
  sessionRepoQuery,
} from '@/lib/api/sessions'
import type { RepoInfo, Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export type RepoBusy = 'pr' | 'commit' | 'push' | 'merge' | 'update' | 'restore' | null

// Shared repo state and actions for the titlebar capsule and the session
// panel: one query, one busy state, one set of mutations against the same
// cache, so both surfaces always agree. Actions resolve true on success so
// callers can chain UI behavior (close a popover, open a URL).
export function useRepoActions(session: Session) {
  const [busy, setBusy] = useState<RepoBusy>(null)
  const toast = useToast()
  const queryClient = useQueryClient()
  const cwd = session.runtime_ref?.cwd
  const repo = useQuery({ ...sessionRepoQuery(session.id), enabled: Boolean(cwd) })
  const info = repo.data

  const web = info?.web_url
  const branch = info?.branch ?? ''
  const onDefaultBranch = Boolean(branch) && branch === info?.default_branch
  // Keep slashes literal so feature/x branches map onto forge URLs.
  const branchPath = branch.split('/').map(encodeURIComponent).join('/')
  const defaultPath = (info?.default_branch ?? '').split('/').map(encodeURIComponent).join('/')
  const compareUrl = info?.default_branch
    ? `${web}/compare/${defaultPath}...${branchPath}?expand=1`
    : `${web}/pull/new/${branchPath}`
  // Cases no automation can fix stay disabled with an explanation; dirty work
  // and a missing upstream are handled by createPR committing/pushing first.
  const prHint = !branch
    ? 'Detached HEAD — check out a branch first'
    : onDefaultBranch
      ? 'Already on the default branch'
      : info?.no_commits && !info.dirty
        ? 'No changes on this branch yet'
        : info?.dirty
          ? 'Commits, pushes, then opens GitHub'
          : !info?.has_upstream
            ? 'Pushes the branch, then opens GitHub'
            : undefined
  const prDisabled = !branch || onDefaultBranch || Boolean(info?.no_commits && !info?.dirty)

  const setRepoData = (next: RepoInfo) => queryClient.setQueryData(keys.sessionRepo(session.id), next)
  const openUrl = (url: string) => {
    // The main process routes window.open to the system browser.
    window.open(url, '_blank', 'noopener')
  }
  const run = async (kind: Exclude<RepoBusy, null>, fn: () => Promise<void>): Promise<boolean> => {
    setBusy(kind)
    try {
      await fn()
      // Commit/merge move what the diff-vs-base view shows; the prefix also
      // covers the changes summary and cached file diffs.
      queryClient.invalidateQueries({ queryKey: keys.sessionRepo(session.id) })
      return true
    } catch (error) {
      toast((error as Error).message, 'danger')
      // The repo may have changed server-side even when the action failed.
      void repo.refetch()
      return false
    } finally {
      setBusy(null)
    }
  }

  const createPR = () =>
    run('pr', async () => {
      let state = info!
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
          ? `Handed off to ${info?.main_branch} — the session now works in the main checkout`
          : `Handed off to ${info?.main_branch} — the agent keeps working in the worktree`,
      )
    })
  const update = () =>
    run('update', async () => {
      setRepoData(await mergeFromMainSessionRepo(session.id))
      toast(`Updated from ${info?.update_branch || info?.main_branch}`)
    })
  const restore = () =>
    run('restore', async () => {
      setRepoData(await restoreSessionWorktree(session.id))
      toast('Worktree restored')
    })

  return {
    cwd,
    info,
    refetch: () => void repo.refetch(),
    busy,
    web,
    branch,
    branchPath,
    onDefaultBranch,
    compareUrl,
    prHint,
    prDisabled,
    openUrl,
    createPR,
    commit,
    push,
    merge,
    update,
    restore,
  }
}
