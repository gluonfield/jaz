import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  CheckCircle2,
  Clock,
  Folder,
  GitBranch,
  GitPullRequest,
  Lock,
  ShieldCheck,
  ShieldOff,
  Users,
  XCircle,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { isCloudApiSession } from '@/lib/api/client'
import { openSafeGitHubUrl } from '@/lib/api/cloudGitHub'
import { useCloudThreadRepos } from '@/lib/api/cloudThreadRepos'
import { spacesQuery, updateThreadUserState } from '@/lib/api/cloudSpaces'
import type { Session } from '@/lib/api/types'
import { useHasCloud, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

// Compact visibility/project badge (`Shared in <Project>`, `<Project>`, or `Private`),
// attached Cloud repository/PR/CI capsule, and Personal Sources opt-in switch.
export function CloudThreadControls({
  session,
  onOpenDiff,
  onOpenOverview,
}: {
  session: Session
  onOpenDiff?: (repo: string) => void
  onOpenOverview?: () => void
}) {
  const hasCloud = useHasCloud()
  const isViewer = useIsCloudViewer()
  const isCloudSession = session.backend === 'cloud' || isCloudApiSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const spaces = useQuery({ ...spacesQuery, enabled: isCloudSession && hasCloud })
  const repoQuery = useCloudThreadRepos(session.id, isCloudSession && hasCloud)
  const isShared = session.visibility === 'space'
  const spaceName = session.space_id
    ? spaces.data?.find((space) => space.id === session.space_id)?.name ?? 'Project'
    : 'Project'

  const [personalSources, setPersonalSources] = useState(Boolean(session.personal_sources_enabled))
  useEffect(() => {
    setPersonalSources(Boolean(session.personal_sources_enabled))
  }, [session.id, session.personal_sources_enabled])

  const togglePersonal = useMutation({
    mutationFn: (enabled: boolean) => updateThreadUserState(session.id, { personal_sources_enabled: enabled }),
    onMutate: (enabled) => {
      const previous = personalSources
      setPersonalSources(enabled)
      return { previous }
    },
    onSuccess: (updated) => {
      setPersonalSources(Boolean(updated.personal_sources_enabled))
      void queryClient.invalidateQueries({ queryKey: keys.session(session.id) })
      void queryClient.invalidateQueries({ queryKey: keys.sessionMessages(session.id) })
    },
    onError: (error: Error, _enabled, context) => {
      if (context) setPersonalSources(context.previous)
      toast(`Could not update Personal Sources: ${error.message}`, 'danger')
    },
  })

  if (!isCloudSession) return null

  const pillClass = `inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors ${
    isShared ? 'bg-primary-soft text-primary hover:bg-primary-soft/80' : 'bg-surface-2 text-ink-2 hover:text-ink'
  }`

  const repos = repoQuery.data ?? []
  const repoInfo = repos.length === 1 ? repos[0] : undefined
  const repoFull = repoInfo?.repo ?? ''
  const changedRepo = repos.find((repo) => repo.dirty || (repo.ahead ?? 0) > 0)
  const repoShort = repoFull ? (repoFull.split('/').at(-1) || repoFull) : ''
  const branch = repoInfo?.branch || repoInfo?.base_branch || ''
  const prUrl = repoInfo?.pr_url || ''
  const prNumber = repoInfo?.pr_number || 0
  const checks = repoInfo?.checks_status || ''

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      {session.space_id ? (
        <Link
          to="/spaces/$spaceId"
          params={{ spaceId: session.space_id }}
          className={pillClass}
          title={isShared ? `Shared in ${spaceName}` : `In private project ${spaceName}`}
        >
          {isShared ? <Users size={11} /> : <Folder size={11} />}
          <span className="max-w-[140px] truncate">{isShared ? `Shared in ${spaceName}` : spaceName}</span>
        </Link>
      ) : (
        <span className={pillClass} title="Private to you">
          <Lock size={11} />
          <span>Private</span>
        </span>
      )}

      {repos.length > 0 ? (
        <button
          type="button"
          onClick={() => {
            if (changedRepo && onOpenDiff) {
              onOpenDiff(changedRepo.repo)
            } else {
              onOpenOverview?.()
            }
          }}
          title={repos.map(({ repo }) => repo).join(', ')}
          className="inline-flex cursor-pointer items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-ink-2 transition-colors hover:bg-surface hover:text-ink"
        >
          <GitBranch size={11} className="shrink-0 text-ink-3" />
          <span className="max-w-[150px] truncate font-mono text-[10.5px]">
            {repos.length > 1 ? `${repos.length} repositories` : branch ? `${repoShort}:${branch}` : repoShort}
          </span>
          {changedRepo && (!repoInfo || repoInfo.dirty) ? (
            <span className="size-1.5 shrink-0 rounded-full bg-running" aria-label={repoInfo ? 'Uncommitted changes' : 'Repository changes'} />
          ) : null}
          {(repoInfo?.ahead ?? 0) > 0 ? (
            <span className="font-mono text-[10px] text-ink-3 tabular-nums">↑{repoInfo?.ahead}</span>
          ) : null}
        </button>
      ) : null}

      {prUrl && prNumber > 0 ? (
        <button
          type="button"
          onClick={() => {
            try {
              openSafeGitHubUrl(prUrl)
            } catch (err) {
              toast(err instanceof Error ? err.message : 'Invalid pull request URL', 'danger')
            }
          }}
          title={`Open PR #${prNumber}${repoInfo?.pr_state ? ` (${repoInfo.pr_state})` : ''}${checks ? ` · checks ${checks}` : ''}`}
          className="inline-flex cursor-pointer items-center gap-1 rounded-full bg-surface px-2 py-0.5 text-[11px] font-medium text-ink-2 ring-1 ring-border/70 transition-colors hover:bg-surface-2 hover:text-ink"
        >
          <GitPullRequest
            size={11}
            className={
              repoInfo?.pr_state === 'merged'
                ? 'text-primary'
                : repoInfo?.pr_state === 'closed'
                  ? 'text-ink-3'
                  : 'text-ok'
            }
          />
          <span className="font-mono text-[10.5px] tabular-nums">#{prNumber}</span>
          {checks === 'success' ? (
            <CheckCircle2 size={11} className="text-ok" aria-label="Checks passing" />
          ) : checks === 'failure' ? (
            <XCircle size={11} className="text-danger" aria-label="Checks failing" />
          ) : checks === 'pending' ? (
            <Clock size={11} className="text-running" aria-label="Checks pending" />
          ) : null}
        </button>
      ) : null}

      {isShared && !isViewer ? (
        <label
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-full bg-surface px-2 py-0.5 text-[11px] text-ink-2 ring-1 ring-border/70 transition-colors duration-150 hover:text-ink"
          title={
            personalSources
              ? 'Personal connectors are enabled for your turns in this shared thread'
              : 'Personal connectors (mailbox, WhatsApp, paired machines) are isolated in this shared thread'
          }
        >
          {personalSources ? (
            <ShieldCheck size={11} className="text-ok" />
          ) : (
            <ShieldOff size={11} className="text-ink-3" />
          )}
          <span className="whitespace-nowrap">Personal Sources</span>
          <Switch
            checked={personalSources}
            disabled={togglePersonal.isPending}
            aria-label="Personal Sources"
            onChange={(checked) => togglePersonal.mutate(checked)}
            className="scale-[0.82]"
          />
        </label>
      ) : null}
    </div>
  )
}
