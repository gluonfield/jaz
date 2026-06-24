import {
  Archive,
  ArchiveRestore,
  ArrowDownToLine,
  ArrowUpFromLine,
  Ban,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Copy,
  ExternalLink,
  FileSearch,
  GitBranch,
  GitMerge,
  GitPullRequest,
  GitPullRequestArrow,
  type LucideIcon,
  LoaderCircle,
} from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useToast } from '@/components/ui/toast'
import { setSessionArchived } from '@/lib/api/sessions'
import { skillsQuery, type SkillInfo } from '@/lib/api/skills'
import type { RepoInfo, Session } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import type { ProviderSubagentView } from '@/lib/providerSubagents'
import type { SendMessageOptions } from '@/lib/sendMessage'
import type { SpawnedThreadView } from '@/lib/spawnedThreads'
import { taskStepState, type TaskSurface } from '@/lib/taskSurface'
import { keys } from '@/lib/query/keys'
import { agentLabel } from '@/lib/agentLabel'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { SidePanelShell } from './SidePanelShell'
import { encodeMention } from './mentionCodec'
import { TaskStepIcon } from './TaskChecklist'
import { useRepoActions } from './useRepoActions'

export const OVERVIEW_PANEL_WIDTH = 300

export function OverviewPanel({
  session,
  progress,
  subagents,
  spawnedThreads,
  working,
  onSend,
}: {
  session: Session
  progress?: TaskSurface
  subagents: ProviderSubagentView[]
  spawnedThreads: SpawnedThreadView[]
  working: boolean
  onSend: (text: string, options?: SendMessageOptions) => void
}) {
  const repo = useRepoActions(session)
  const showGit = Boolean(repo.cwd && (repo.info?.git || repo.info?.worktree_missing))
  return (
    <SidePanelShell width={OVERVIEW_PANEL_WIDTH} variant="hug" className="gap-6 px-4 py-4">
      {spawnedThreads.length ? <ThreadsSection threads={spawnedThreads} /> : null}
      {subagents.length ? <SubagentsSection subagents={subagents} /> : null}
      {progress ? <ProgressSection progress={progress} working={working} /> : null}
      {showGit ? <GitSection repo={repo} /> : null}
      <ManageSection session={session} repo={repo} onSend={onSend} />
    </SidePanelShell>
  )
}

type SubagentStatus = 'working' | 'completed' | 'failed' | 'cancelled'
type ThreadStatus = 'running' | 'idle' | 'failed' | 'cancelled'
type OverviewRunStatus = { label: string; className: string; Icon: LucideIcon; spin?: boolean }

const SUBAGENT_STATUS: Record<SubagentStatus, OverviewRunStatus> = {
  working: { label: 'working', className: 'text-running', Icon: LoaderCircle, spin: true },
  completed: { label: 'completed', className: 'text-primary', Icon: CheckCircle2 },
  failed: { label: 'failed', className: 'text-danger', Icon: CircleAlert },
  cancelled: { label: 'cancelled', className: 'text-ink-3', Icon: Ban },
}

const THREAD_STATUS: Record<ThreadStatus, OverviewRunStatus> = {
  running: { label: 'running', className: 'text-running', Icon: LoaderCircle, spin: true },
  idle: { label: 'idle', className: 'text-primary', Icon: CheckCircle2 },
  failed: { label: 'failed', className: 'text-danger', Icon: CircleAlert },
  cancelled: { label: 'cancelled', className: 'text-ink-3', Icon: Ban },
}

function SectionHeader({ children }: { children: ReactNode }) {
  return <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">{children}</p>
}

function ThreadsSection({ threads }: { threads: SpawnedThreadView[] }) {
  return (
    <section>
      <div className="mb-2">
        <SectionHeader>Threads</SectionHeader>
      </div>
      <ul className="flex flex-col gap-1.5">
        {threads.map((thread) => (
          <ThreadRow key={thread.key} thread={thread} />
        ))}
      </ul>
    </section>
  )
}

function ThreadRow({ thread }: { thread: SpawnedThreadView }) {
  const status = THREAD_STATUS[threadStatus(thread.state)]
  const title = firstText(thread.title, thread.slug) || 'Thread'
  const detail = threadDetail(thread)
  return (
    <li className="min-w-0 rounded-md">
      <Link
        to="/sessions/$sessionId"
        params={{ sessionId: thread.id }}
        title={`Open ${title}`}
        className="flex min-h-10 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left transition-colors duration-150 hover:bg-surface-2"
      >
        <OverviewRunRowContent
          agent={thread.agent}
          title={title}
          detail={detail}
          status={status}
          trailing={<ChevronRight size={13} className="shrink-0 text-ink-3" aria-hidden />}
        />
      </Link>
    </li>
  )
}

function threadStatus(state: string | undefined): ThreadStatus {
  const normalized = state?.toLowerCase()
  if (normalized === 'idle' || normalized === 'completed') return 'idle'
  if (normalized === 'failed' || normalized === 'errored' || normalized === 'error') return 'failed'
  if (normalized === 'cancelled' || normalized === 'canceled' || normalized === 'interrupted') return 'cancelled'
  return 'running'
}

function threadDetail(thread: SpawnedThreadView): string {
  const model = compactModel(thread.model)
  return [agentLabel(thread.agent), model ? withReasoningEffort(model, thread.reasoning_effort) : '']
    .filter(Boolean)
    .join(' · ')
}

function compactModel(model?: string): string {
  if (!model) return ''
  const parts = model.split('/').filter(Boolean)
  return parts.at(-1) ?? model
}

function withReasoningEffort(model: string, effort?: string): string {
  return effort ? `${model}/${effort}` : model
}

function OverviewRunRowContent({
  agent,
  title,
  detail,
  status,
  trailing,
}: {
  agent?: string
  title: string
  detail?: string
  status: OverviewRunStatus
  trailing?: ReactNode
}) {
  return (
    <>
      <AgentAvatar agent={agent} size={17} />
      <span className="flex min-w-0 flex-1 flex-col justify-center">
        <span className="block truncate text-[13px] font-medium leading-5 text-ink" title={title}>
          {title}
        </span>
        {detail ? (
          <span className="mt-0.5 block truncate text-[12px] leading-snug text-ink-3" title={detail}>
            {detail}
          </span>
        ) : null}
      </span>
      <span
        className={`inline-flex h-5 w-5 shrink-0 items-center justify-center ${status.className}`}
        title={status.label}
        aria-label={status.label}
      >
        <status.Icon size={13} className={status.spin ? 'animate-spin' : ''} aria-hidden />
      </span>
      {trailing}
    </>
  )
}

function SubagentsSection({ subagents }: { subagents: ProviderSubagentView[] }) {
  return (
    <section>
      <div className="mb-2">
        <SectionHeader>Subagents</SectionHeader>
      </div>
      <ul className="flex flex-col gap-1.5">
        {subagents.map((subagent) => (
          <SubagentRow key={subagent.key} subagent={subagent} />
        ))}
      </ul>
    </section>
  )
}

function SubagentRow({ subagent }: { subagent: ProviderSubagentView }) {
  const [expanded, setExpanded] = useState(false)
  const status = SUBAGENT_STATUS[subagentStatus(subagent.status)]
  const title = subagentTitle(subagent)
  const detail = subagentSummary(subagent.summary)
  const prompt = subagent.prompt?.trim() ?? ''
  return (
    <li className="min-w-0 rounded-md">
      <button
        type="button"
        disabled={!prompt}
        title={prompt ? (expanded ? 'Hide prompt' : 'Show prompt') : undefined}
        onClick={() => setExpanded((open) => !open)}
        className="flex min-h-10 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left transition-colors duration-150 enabled:cursor-pointer enabled:hover:bg-surface-2 disabled:cursor-default"
      >
        <OverviewRunRowContent
          agent={subagent.provider}
          title={title}
          detail={detail && detail !== title ? detail : undefined}
          status={status}
          trailing={
            prompt ? (
              <ChevronDown
                size={13}
                className={`shrink-0 text-ink-3 transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}
                aria-hidden
              />
            ) : null
          }
        />
      </button>
      {expanded && prompt ? (
        <p className="ml-[25px] max-h-28 overflow-y-auto whitespace-pre-wrap px-1 pb-1 text-[11px] leading-snug text-ink-3">
          {prompt}
        </p>
      ) : null}
    </li>
  )
}

function subagentStatus(status: string | undefined): SubagentStatus {
  const normalized = status?.toLowerCase()
  if (normalized === 'completed' || normalized === 'shutdown' || normalized === 'closed') return 'completed'
  if (normalized === 'failed' || normalized === 'errored' || normalized === 'error' || normalized === 'not_found') {
    return 'failed'
  }
  if (normalized === 'cancelled' || normalized === 'canceled' || normalized === 'interrupted') return 'cancelled'
  return 'working'
}

function subagentTitle(subagent: ProviderSubagentView): string {
  return firstText(subagent.name, subagent.role) || 'Subagent'
}

function subagentSummary(summary: string | undefined): string | undefined {
  const text = summary?.trim()
  if (!text) return undefined
  switch (text.toLowerCase()) {
    case 'spawned':
    case 'working':
    case 'waiting':
    case 'wait finished':
    case 'responded':
      return undefined
    default:
      return text
  }
}

function firstText(...values: Array<string | undefined>): string {
  return values.find((value) => value?.trim())?.trim() ?? ''
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

function ProgressSection({ progress, working }: { progress: TaskSurface; working: boolean }) {
  const [open, setOpen] = useState(true)
  const entries = progress.entries ?? []
  const states = entries.map(taskStepState)
  const showSteps = states.some(Boolean)
  const completedCount = states.filter((state) => state === 'completed').length
  return (
    <section>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md text-left transition-colors hover:text-ink"
      >
        <SectionHeader>
          {progress.title}
          {showSteps ? (
            <span className="ml-2 font-mono normal-case tracking-normal">
              {completedCount}/{entries.length}
            </span>
          ) : null}
        </SectionHeader>
        <ChevronDown
          size={13}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ease-out ${open ? '' : '-rotate-90'}`}
          aria-hidden
        />
      </button>
      {open ? (
        <>
          {entries.length ? (
            <ul className="mt-2.5 flex flex-col gap-2">
              {entries.map((entry, index) => {
                const state = states[index]
                return (
                  <li
                    key={`${entry.content}-${index}`}
                    className="flex min-w-0 items-start gap-2 text-[13px] leading-snug text-ink-2"
                  >
                    {showSteps ? (
                      <span className="mt-[2px] shrink-0" title={state}>
                        <TaskStepIcon state={state ?? 'pending'} active={working} />
                      </span>
                    ) : null}
                    <span className={`min-w-0 flex-1 ${state === 'completed' ? 'opacity-50' : ''}`}>
                      {entry.content}
                    </span>
                  </li>
                )
              })}
            </ul>
          ) : (
            <p className="mt-2.5 text-[12px] italic text-ink-3">(no steps provided)</p>
          )}
        </>
      ) : null}
    </section>
  )
}

function ManageSection({
  session,
  repo,
  onSend,
}: {
  session: Session
  repo: ReturnType<typeof useRepoActions>
  onSend: (text: string, options?: SendMessageOptions) => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const info = repo.info
  const showCodeReview = canReviewSession(info)
  const skills = useQuery({ ...skillsQuery(), enabled: showCodeReview })
  const archive = useMutation({
    mutationFn: (archived: boolean) => setSessionArchived(session.id, archived),
    onSuccess: (_, archived) => toast(archived ? 'Archived thread' : 'Restored thread'),
    onError: (error: Error, archived) =>
      toast(`Couldn't ${archived ? 'archive' : 'restore'}: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.session(session.id) })
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(session.id) })
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
    },
  })
  const sendCodeReview = async () => {
    let catalog = skills.data ?? []
    let skill = catalog.find((candidate) => candidate.name === CODE_REVIEW_SKILL)
    if (!skill) {
      catalog = (await skills.refetch()).data ?? catalog
      skill = catalog.find((candidate) => candidate.name === CODE_REVIEW_SKILL)
    }
    if (!skill) {
      toast(`Couldn't find skill: ${CODE_REVIEW_SKILL}`, 'danger')
      return
    }
    onSend(codeReviewPrompt(skill))
  }

  return (
    <section className="flex flex-col gap-0.5">
      <div className="mb-1.5">
        <SectionHeader>Manage</SectionHeader>
      </div>
      {showCodeReview ? (
        <ActionRow
          icon={skills.isFetching ? LoaderCircle : FileSearch}
          spin={skills.isFetching}
          disabled={repo.busy !== null || skills.isFetching}
          hint="Review this session's code changes"
          onClick={() => void sendCodeReview()}
        >
          Code Review
        </ActionRow>
      ) : null}
      <ActionRow
        icon={archive.isPending ? LoaderCircle : session.archived ? ArchiveRestore : Archive}
        spin={archive.isPending}
        disabled={archive.isPending}
        hint={session.archived ? 'Restores this thread and its children' : 'Archives this thread and its children'}
        onClick={() => archive.mutate(!session.archived)}
      >
        {session.archived ? 'Unarchive thread' : 'Archive thread'}
      </ActionRow>
    </section>
  )
}

function canHandoffToMain(info: RepoInfo | undefined): boolean {
  return Boolean(info?.is_worktree && info.main_branch && (info.dirty || !info.no_commits))
}

function canReviewSession(info: RepoInfo | undefined): boolean {
  if (!info?.git || info.worktree_missing) return false
  return Boolean(info.dirty || info.needs_push || canHandoffToMain(info) || branchHasCommits(info))
}

function branchHasCommits(info: RepoInfo): boolean {
  return Boolean(info.branch && info.default_branch && info.branch !== info.default_branch && info.no_commits === false)
}

const CODE_REVIEW_SKILL = 'thermo-nuclear-code-quality-review'

function codeReviewPrompt(skill: SkillInfo): string {
  return encodeMention('$', skill.name, skill.path)
}

function GitSection({ repo }: { repo: ReturnType<typeof useRepoActions> }) {
  const { info, busy, web, branch, branchPath } = repo
  const branchLabel = branch || 'detached'
  const [copied, setCopied] = useState(false)
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reduceMotion = useReducedMotion()

  useEffect(() => {
    setCopied(false)
  }, [branchLabel])

  useEffect(() => () => {
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
  }, [])

  if (!info) return null
  if (info.worktree_missing) {
    return (
      <section className="flex flex-col gap-0.5">
        <div className="mb-1.5">
          <SectionHeader>Worktree</SectionHeader>
        </div>
        <div className="flex h-7 items-center gap-2 px-2.5 text-[13px] text-ink-2">
          <GitBranch size={13} className="shrink-0 text-ink-3" />
          <span className="min-w-0 flex-1 truncate font-mono text-[12px]">
            {info.worktree_branch || branch || 'removed'}
          </span>
        </div>
        <p className="px-2.5 py-1 text-[12px] leading-snug text-ink-3">Worktree removed to save space.</p>
        <ActionRow
          icon={busy === 'restore' ? LoaderCircle : ArchiveRestore}
          spin={busy === 'restore'}
          disabled={busy !== null || !info.worktree_restorable}
          hint={info.worktree_restorable ? 'Restores the saved session branch' : 'Saved branch is unavailable'}
          onClick={() => void repo.restore()}
        >
          {busy === 'restore' ? 'Restoring…' : 'Restore worktree'}
        </ActionRow>
      </section>
    )
  }

  const copyBranch = async () => {
    if (!(await writeClipboard(branchLabel))) return
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    setCopied(true)
    copiedTimer.current = setTimeout(() => setCopied(false), 1500)
  }
  const changes = info.dirty
    ? { color: 'bg-running', label: 'Uncommitted changes' }
    : info.needs_push
      ? { color: 'bg-primary', label: 'Unpushed commits' }
      : { color: 'bg-ok', label: 'Working tree clean' }
  return (
    <>
      <section className="flex flex-col gap-0.5">
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <SectionHeader>Git</SectionHeader>
          {web ? (
            <span className="min-w-0 truncate text-[11px] text-ink-3">
              {info.owner}/{info.repo}
            </span>
          ) : null}
        </div>
        <button
          type="button"
          aria-label={copied ? `Copied branch name ${branchLabel}` : `Copy branch name ${branchLabel}`}
          title={copied ? 'Copied' : 'Copy branch name'}
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
        <div className="flex h-7 items-center gap-2 px-2.5 text-[13px] text-ink-2">
          <span className={`size-[9px] shrink-0 rounded-full ${changes.color} mx-0.5`} aria-hidden />
          <span className="min-w-0 flex-1 truncate">{changes.label}</span>
        </div>
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
        {info.is_worktree && info.main_branch && (info.behind ?? 0) > 0 ? (
          <ActionRow
            icon={busy === 'update' ? LoaderCircle : ArrowDownToLine}
            spin={busy === 'update'}
            disabled={busy !== null}
            hint={`Commits this session's work, then merges the latest ${info.main_branch} into this worktree`}
            onClick={() => void repo.update()}
          >
            {busy === 'update' ? 'Updating…' : `Update from ${info.main_branch}`}
          </ActionRow>
        ) : null}
        {canHandoffToMain(info) ? (
          <ActionRow
            icon={busy === 'merge' ? LoaderCircle : GitMerge}
            spin={busy === 'merge'}
            disabled={busy !== null}
            hint={`Commits this session's work and hands its branch off to ${info.main_branch}`}
            onClick={() => void repo.merge()}
          >
            {busy === 'merge' ? 'Handing off…' : `Hand off to ${info.main_branch}`}
          </ActionRow>
        ) : null}
      </section>

      <section className="flex flex-col gap-0.5">
        <div className="mb-1.5">
          <SectionHeader>GitHub</SectionHeader>
        </div>
        {web ? (
          <>
            <ActionRow
              icon={busy === 'pr' ? LoaderCircle : GitPullRequestArrow}
              spin={busy === 'pr'}
              disabled={repo.prDisabled || busy !== null}
              hint={repo.prHint}
              onClick={() => void repo.createPR()}
            >
              {busy === 'pr' ? 'Creating pull request…' : 'Create pull request'}
            </ActionRow>
            <ActionRow icon={GitPullRequest} disabled={busy !== null} onClick={() => repo.openUrl(`${web}/pulls`)}>
              Pull requests
            </ActionRow>
            {branch ? (
              <ActionRow
                icon={GitBranch}
                disabled={busy !== null}
                onClick={() => repo.openUrl(`${web}/tree/${branchPath}`)}
              >
                View branch
              </ActionRow>
            ) : null}
            <ActionRow icon={ExternalLink} disabled={busy !== null} onClick={() => repo.openUrl(web)}>
              Open repository
            </ActionRow>
          </>
        ) : (
          <p className="px-2.5 text-[12px] leading-snug text-ink-3">
            No GitHub remote configured.
            {info.remote_url ? (
              <span className="mt-1 block truncate font-mono text-[11px]">{info.remote_url}</span>
            ) : null}
          </p>
        )}
      </section>
    </>
  )
}
