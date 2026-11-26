import { useNavigate } from '@tanstack/react-router'
import {
  ArrowUpFromLine,
  Check,
  CheckCircle2,
  Clock,
  Copy,
  ExternalLink,
  FolderGit2,
  GitBranch,
  GitPullRequest,
  GitPullRequestArrow,
  LoaderCircle,
  LogIn,
  Puzzle,
  RefreshCw,
  XCircle,
} from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ThreadRepoPicker } from '@/components/cloud/GitHubRepoPicker'
import { withGitHubPluginSearch } from '@/lib/api/cloudGitHub'
import type { Session } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import { SectionHeader } from './OverviewRuns'
import { useCloudThreadRepoActions } from './useCloudThreadRepoActions'

function CloudRepoActionRow({
  icon: Icon,
  onClick,
  disabled,
  hint,
  spin,
  children,
}: {
  icon: typeof GitBranch
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

function ChecksRow({ status }: { status: string }) {
  if (!status) return null
  if (status === 'success') {
    return (
      <p className="flex items-center gap-1.5 text-ok">
        <CheckCircle2 size={12} className="shrink-0" />
        Checks passing
      </p>
    )
  }
  if (status === 'failure') {
    return (
      <p className="flex items-center gap-1.5 text-danger">
        <XCircle size={12} className="shrink-0" />
        Checks failing
      </p>
    )
  }
  return (
    <p className="flex items-center gap-1.5 text-running">
      <Clock size={12} className="shrink-0" />
      Checks pending
    </p>
  )
}

export function CloudGitSection({
  session,
  onOpenDiff,
}: {
  session: Session
  onOpenDiff?: () => void
}) {
  const repo = useCloudThreadRepoActions(session)
  const navigate = useNavigate()
  const [copied, setCopied] = useState(false)
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reduceMotion = useReducedMotion()
  const openGitHubPlugin = () => void navigate({ to: '.', search: withGitHubPluginSearch })

  const branchLabel = repo.branch || repo.baseBranch || 'main'
  const allowPersonal = session.visibility !== 'space' || Boolean(session.personal_sources_enabled)

  useEffect(() => {
    setCopied(false)
  }, [branchLabel])

  useEffect(
    () => () => {
      if (copiedTimer.current) clearTimeout(copiedTimer.current)
    },
    [],
  )

  if (repo.error) {
    return (
      <section className="flex flex-col gap-1">
        <SectionHeader>Git</SectionHeader>
        <p className="px-2.5 text-[12px] leading-snug text-danger">
          Repository status unavailable.
        </p>
        <CloudRepoActionRow icon={RefreshCw} onClick={repo.refetch}>
          Retry repository status
        </CloudRepoActionRow>
      </section>
    )
  }

  if (!repo.attached) {
    if (repo.isViewer || repo.isLoading) return null
    return (
      <section className="flex flex-col gap-1.5">
        <SectionHeader>Repository</SectionHeader>
        <div className="px-1">
          <ThreadRepoPicker
            repo=""
            baseBranch=""
            placement="below"
            placeholder="Attach repository"
            disabled={repo.busy !== null}
            includePersonal={allowPersonal}
            onChange={({ repo: nextRepo, baseBranch }) => {
              if (nextRepo) void repo.attachRepo(nextRepo, baseBranch)
            }}
          />
        </div>
      </section>
    )
  }

  const copyBranch = async () => {
    if (!(await writeClipboard(branchLabel))) return
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    setCopied(true)
    copiedTimer.current = setTimeout(() => setCopied(false), 1500)
  }

  const branchPath = branchLabel.split('/').map(encodeURIComponent).join('/')

  return (
    <>
      <section className="flex flex-col gap-0.5">
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <SectionHeader>Git</SectionHeader>
          <span className="min-w-0 truncate text-[11px] text-ink-3" title={repo.repo}>
            {repo.repo}
          </span>
        </div>

        <button
          type="button"
          aria-label={copied ? `Copied branch name ${branchLabel}` : `Copy branch name ${branchLabel}`}
          title={copied ? 'Copied' : `Copy branch name (${repo.baseBranch ? `base: ${repo.baseBranch}` : 'default'})`}
          onClick={() => void copyBranch()}
          className="group flex h-7 w-full cursor-pointer items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <GitBranch size={13} className="shrink-0 text-ink-3" />
          <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{branchLabel}</span>
          <span className="grid size-3 shrink-0 place-items-center">
            <AnimatePresence initial={false} mode="popLayout">
              <motion.span
                key={copied ? 'copied' : 'copy'}
                initial={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
                animate={reduceMotion ? { opacity: 1 } : { opacity: 1, scale: 1, filter: 'blur(0px)' }}
                exit={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
                transition={reduceMotion ? { duration: 0.12 } : { type: 'spring', duration: 0.3, bounce: 0 }}
                className="grid size-3 place-items-center"
              >
                {copied ? (
                  <Check size={12} className="text-primary" />
                ) : (
                  <Copy size={12} className="text-ink-3 opacity-70 transition-opacity group-hover:opacity-100" />
                )}
              </motion.span>
            </AnimatePresence>
          </span>
        </button>

        {repo.dirty || repo.ahead > 0 || repo.behind > 0 || repo.checksStatus || repo.prNumber > 0 ? (
          <div className="space-y-1 px-2.5 py-1 text-[12px] leading-snug text-ink-3">
            {repo.dirty ? <p className="text-running">Uncommitted workspace changes</p> : null}
            {repo.ahead > 0 || repo.behind > 0 ? (
              <p>
                {repo.ahead > 0 ? `${repo.ahead} ahead` : ''}
                {repo.ahead > 0 && repo.behind > 0 ? ' · ' : ''}
                {repo.behind > 0 ? `${repo.behind} behind` : ''}
                {repo.baseBranch ? ` of ${repo.baseBranch}` : ''}
              </p>
            ) : null}
            {repo.prNumber > 0 ? (
              <p>
                PR #{repo.prNumber}
                {repo.prState ? ` · ${repo.prState}` : ''}
              </p>
            ) : null}
            <ChecksRow status={repo.checksStatus} />
          </div>
        ) : null}

        {onOpenDiff ? (
          <CloudRepoActionRow icon={FolderGit2} onClick={onOpenDiff}>
            View changes
          </CloudRepoActionRow>
        ) : null}

        {!repo.isViewer ? (
          <CloudRepoActionRow
            icon={repo.busy === 'push' ? LoaderCircle : ArrowUpFromLine}
            spin={repo.busy === 'push'}
            disabled={repo.busy !== null}
            hint="Commits workspace edits and pushes this branch to GitHub"
            onClick={() => void repo.push()}
          >
            {repo.busy === 'push' ? 'Pushing…' : 'Push branch'}
          </CloudRepoActionRow>
        ) : null}

        {!repo.isViewer ? (
          <div className="pt-1">
            <ThreadRepoPicker
              repo={repo.repo}
              baseBranch={repo.baseBranch}
              placement="below"
              placeholder="Change repository"
              disabled={repo.busy !== null}
              includePersonal={allowPersonal}
              onChange={({ repo: nextRepo, baseBranch }) => {
                if (nextRepo !== repo.repo || baseBranch !== repo.baseBranch) {
                  void repo.attachRepo(nextRepo, baseBranch)
                }
              }}
            />
          </div>
        ) : null}
      </section>

      <section className="flex flex-col gap-0.5">
        <div className="mb-1.5">
          <SectionHeader>GitHub</SectionHeader>
        </div>

        {repo.prUrl ? (
          <CloudRepoActionRow
            icon={GitPullRequest}
            disabled={repo.busy !== null}
            hint={repo.prHint}
            onClick={() => void repo.createOrOpenPR()}
          >
            {repo.prNumber > 0 ? `Open pull request #${repo.prNumber}` : 'Open pull request'}
          </CloudRepoActionRow>
        ) : !repo.isViewer ? (
          <CloudRepoActionRow
            icon={repo.busy === 'pr' ? LoaderCircle : GitPullRequestArrow}
            spin={repo.busy === 'pr'}
            disabled={repo.prDisabled || repo.busy !== null}
            hint={repo.prHint}
            onClick={() => void repo.createOrOpenPR()}
          >
            {repo.busy === 'pr' ? 'Creating pull request…' : 'Create pull request'}
          </CloudRepoActionRow>
        ) : null}

        {repo.connectRequired && !repo.isViewer ? (
          <>
            <CloudRepoActionRow
              icon={repo.connectingGitHub ? LoaderCircle : LogIn}
              spin={repo.connectingGitHub}
              disabled={repo.connectingGitHub}
              onClick={repo.connectGitHub}
            >
              Connect GitHub account
            </CloudRepoActionRow>
            <CloudRepoActionRow icon={Puzzle} onClick={openGitHubPlugin}>
              Manage GitHub in Plugins
            </CloudRepoActionRow>
          </>
        ) : null}

        {repo.compareUrl ? (
          <CloudRepoActionRow
            icon={ExternalLink}
            onClick={() => repo.openUrl(repo.compareUrl)}
          >
            Open GitHub compare
          </CloudRepoActionRow>
        ) : null}

        {repo.webUrl ? (
          <>
            <CloudRepoActionRow
              icon={GitPullRequest}
              onClick={() => repo.openUrl(`${repo.webUrl}/pulls`)}
            >
              Pull requests
            </CloudRepoActionRow>
            <CloudRepoActionRow
              icon={GitBranch}
              onClick={() => repo.openUrl(`${repo.webUrl}/tree/${branchPath}`)}
            >
              View branch
            </CloudRepoActionRow>
            <CloudRepoActionRow
              icon={ExternalLink}
              onClick={() => repo.openUrl(repo.webUrl)}
            >
              Open repository
            </CloudRepoActionRow>
          </>
        ) : null}
      </section>
    </>
  )
}
