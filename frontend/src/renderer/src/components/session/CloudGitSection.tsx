import { useQuery } from '@tanstack/react-query'
import {
  ArrowUpFromLine,
  Copy,
  ExternalLink,
  FolderGit2,
  GitBranch,
  GitPullRequest,
  LoaderCircle,
  LogIn,
  RefreshCw,
  X,
} from 'lucide-react'
import { useState } from 'react'
import { AddGitHubRepository, GitHubBranchField } from '@/components/cloud/GitHubRepoPicker'
import { SectionHeader } from '@/components/session/OverviewRuns'
import { useCloudThreadRepoActions } from '@/components/session/useCloudThreadRepoActions'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import {
  githubRepoWebUrl,
  threadRepoStatuses,
  useCloudThreadRepo,
  type CloudThreadRepoStatus,
} from '@/lib/api/cloudGitHub'
import { sessionQuery } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import { isCloudViewerRole, useCloudSession } from '@/lib/cloudSession'

export function CloudGitSection({ session, onOpenDiff }: {
  session: Session
  onOpenDiff?: (repo: string) => void
}) {
  const cloud = useCloudSession()
  const isViewer = isCloudViewerRole(cloud.user?.role)
  const isAdmin = cloud.user?.role === 'admin'
  const thread = useQuery({
    ...sessionQuery(session.id),
    enabled: Boolean(cloud.user) && !isAdmin && !isViewer && !session.owner_user_id,
  })
  const owner = session.owner_user_id ?? thread.data?.owner_user_id
  const canManage = Boolean(cloud.user && !isViewer && (isAdmin || cloud.user.id === owner))
  const query = useCloudThreadRepo(session.id)
  const actions = useCloudThreadRepoActions(session.id)
  const repos = threadRepoStatuses(query.data)

  return (
    <section className="flex flex-col gap-2" aria-busy={actions.busy || query.isPending}>
      <SectionHeader>Git</SectionHeader>
      {query.isError ? (
        <div className="px-2">
          <p className="text-[12px] text-danger">Repository status unavailable.</p>
          <Button variant="ghost" size="sm" onClick={() => void query.refetch()}><RefreshCw size={12} /> Retry</Button>
        </div>
      ) : query.isPending ? (
        <p className="flex items-center gap-2 px-2 text-[12px] text-ink-3"><LoaderCircle size={12} className="animate-spin" /> Loading repositories…</p>
      ) : (
        <>
          {repos.map((repo) => (
            <GitRepository
              key={repo.repo}
              repo={repo}
              canManage={canManage}
              isViewer={isViewer}
              actions={actions}
              onOpenDiff={onOpenDiff}
            />
          ))}
          {canManage ? (
            <AddGitHubRepository
              value={repos}
              includePersonal={session.visibility !== 'space' || Boolean(session.personal_sources_enabled)}
              disabled={actions.busy}
              onSelect={(repo) => actions.saveRepo(repo, `Attached ${repo.repo}`)}
            />
          ) : repos.length === 0 ? (
            <p className="px-2 text-[12px] text-ink-3">No repositories attached.</p>
          ) : null}
          {actions.connectRequired && !isViewer ? (
            <Button className="self-start" variant="ghost" size="sm" onClick={actions.connect} disabled={actions.connecting}>
              <LogIn size={12} /> Connect GitHub
            </Button>
          ) : null}
        </>
      )}
    </section>
  )
}

function GitRepository({ repo, actions, canManage, isViewer, onOpenDiff }: {
  repo: CloudThreadRepoStatus
  actions: ReturnType<typeof useCloudThreadRepoActions>
  canManage: boolean
  isViewer: boolean
  onOpenDiff?: (repo: string) => void
}) {
  const [baseBranch, setBaseBranch] = useState<string | null>(null)
  const [confirmDetach, setConfirmDetach] = useState(false)
  const toast = useToast()
  const branch = repo.branch || repo.base_branch || ''
  const base = repo.base_branch || ''
  const webUrl = githubRepoWebUrl(repo.repo)
  const canOpenPR = Boolean(repo.pr_url || (!isViewer && branch && branch !== base))
  const mount = repo.mount_dir ? `/workspace/${repo.mount_dir}` : undefined

  async function copyBranch() {
    if (await writeClipboard(branch)) toast(`Copied ${branch}`)
  }

  return (
    <div className="space-y-1.5 border-b border-border px-2 pb-2.5">
      <div className="flex items-center justify-between gap-1.5">
        {webUrl ? (
          <button type="button" title={repo.repo} onClick={() => actions.openUrl(webUrl)} className="flex min-w-0 cursor-pointer items-center gap-1.5 text-left text-[12px] font-medium text-ink hover:underline">
            <span className="truncate">{repo.repo}</span><ExternalLink size={11} className="shrink-0 text-ink-3" />
          </button>
        ) : <span className="truncate text-[12px] font-medium text-ink" title={repo.repo}>{repo.repo}</span>}
        {canManage ? (
          <button type="button" aria-label={`Detach ${repo.repo}`} title="Detach repository" className="grid size-7 shrink-0 cursor-pointer place-items-center rounded-control text-ink-3 hover:bg-surface-2 hover:text-ink disabled:opacity-50" disabled={actions.busy} onClick={() => setConfirmDetach(!confirmDetach)}>
            <X size={13} />
          </button>
        ) : null}
      </div>
      {mount ? <p className="truncate font-mono text-[11px] text-ink-3" title={mount}>{mount}</p> : null}
      {branch ? (
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => void copyBranch()} className="flex max-w-full cursor-pointer items-center gap-1.5 font-mono text-[12px] text-ink-2 hover:text-ink" title={`Copy branch name ${branch}`}>
            <GitBranch size={12} className="shrink-0" />
            <span className="truncate">{branch}</span>
            <Copy size={11} className="shrink-0 text-ink-3" />
          </button>
          {webUrl ? (
            <button
              type="button"
              aria-label={`View ${branch} on GitHub`}
              title="View branch on GitHub"
              className="shrink-0 cursor-pointer text-ink-3 hover:text-ink"
              onClick={() => actions.openUrl(`${webUrl}/tree/${encodeURIComponent(branch)}`)}
            >
              <ExternalLink size={11} />
            </button>
          ) : null}
        </div>
      ) : null}
      <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-[11px] text-ink-3 tabular-nums">
        {base ? <span title={base}>Base · {base}</span> : null}
        {repo.dirty ? <span className="text-running">Uncommitted changes</span> : null}
        {Boolean(repo.ahead) ? <span>{repo.ahead} ahead</span> : null}
        {Boolean(repo.behind) ? <span>{repo.behind} behind</span> : null}
        {repo.checks_status ? <span>Checks {repo.checks_status === 'success' ? 'passing' : repo.checks_status === 'failure' ? 'failing' : repo.checks_status}</span> : null}
        {repo.pr_state ? <span>PR {repo.pr_state}</span> : null}
      </div>
      {canManage && confirmDetach ? (
        <div className="space-y-1">
          <p className="text-[12px] text-ink-2">Detach this repository from the thread?</p>
          <div className="flex gap-1">
            <Button variant="ghost" size="sm" onClick={() => setConfirmDetach(false)}>Cancel</Button>
            <Button variant="danger" size="sm" disabled={actions.busy} onClick={() => {
              setConfirmDetach(false)
              actions.removeRepo(repo)
            }}>Detach</Button>
          </div>
        </div>
      ) : null}
      {canManage && baseBranch !== null ? (
        <form className="space-y-1.5" onSubmit={(event) => {
          event.preventDefault()
          const next = baseBranch.trim()
          if (!next || next === base || actions.busy) return
          actions.saveRepo({ repo: repo.repo, mount_dir: repo.mount_dir, base_branch: next }, `Updated base branch for ${repo.repo}`)
          setBaseBranch(null)
        }}>
          <GitHubBranchField repo={repo.repo} label="Base branch" value={baseBranch} disabled={actions.busy} onChange={setBaseBranch} />
          <div className="flex gap-1">
            <Button variant="ghost" size="sm" onClick={() => setBaseBranch(null)}>Cancel</Button>
            <Button type="submit" variant="secondary" size="sm" disabled={actions.busy || !baseBranch.trim() || baseBranch.trim() === base}>Save base branch</Button>
          </div>
        </form>
      ) : null}
      <div className="flex flex-wrap gap-1">
        {onOpenDiff ? (
          <Button variant="ghost" size="sm" onClick={() => onOpenDiff(repo.repo)}><FolderGit2 size={12} /> Changes</Button>
        ) : null}
        {!isViewer ? (
          <Button variant="ghost" size="sm" disabled={actions.busy || !branch} onClick={() => actions.push(repo)}><ArrowUpFromLine size={12} /> Push</Button>
        ) : null}
        {canOpenPR ? (
          <Button variant="ghost" size="sm" disabled={actions.busy} onClick={() => actions.openPR(repo)}>
            <GitPullRequest size={12} /> {repo.pr_url ? (repo.pr_number ? `Open PR #${repo.pr_number}` : 'Open PR') : 'Create PR'}
          </Button>
        ) : null}
        {webUrl ? (
          <Button variant="ghost" size="sm" onClick={() => actions.openUrl(`${webUrl}/pulls`)}>Pull requests</Button>
        ) : null}
        {canManage && baseBranch === null ? (
          <Button variant="ghost" size="sm" disabled={actions.busy} onClick={() => setBaseBranch(base)}>Edit base branch</Button>
        ) : null}
      </div>
    </div>
  )
}
