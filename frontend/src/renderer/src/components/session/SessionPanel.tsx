import {
  Archive,
  ArrowLeftRight,
  ArrowUpFromLine,
  Check,
  ChevronDown,
  ExternalLink,
  FileDiff,
  GitBranch,
  GitPullRequest,
  GitPullRequestArrow,
  type LucideIcon,
  LoaderCircle,
} from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useToast } from '@/components/ui/toast'
import { sessionRepoChangesQuery, setSessionArchived } from '@/lib/api/sessions'
import { fileKey, type Session } from '@/lib/api/types'
import { planStepState, type PlanSurface } from '@/lib/planSurface'
import { keys } from '@/lib/query/keys'
import { DiffModal } from './DiffModal'
import { FileCounts } from './DiffView'
import { MessageMarkdown } from './MessageMarkdown'
import { PlanStepIcon } from './Transcript'
import { useRepoActions } from './useRepoActions'

export const SESSION_PANEL_WIDTH = 300

export function SessionPanel({
  session,
  plan,
  working,
  visible = true,
}: {
  session: Session
  plan?: PlanSurface
  working: boolean
  // The page keeps the panel mounted while collapsed (the wrapper animates to
  // width 0); visible=false pauses work that should not run off-screen.
  visible?: boolean
}) {
  const repo = useRepoActions(session)
  const showGit = Boolean(repo.cwd && repo.info?.git)
  return (
    <aside
      style={{ width: SESSION_PANEL_WIDTH }}
      className="flex h-full shrink-0 flex-col gap-6 overflow-y-auto border-l border-border px-4 py-4"
    >
      {plan ? <PlanSection plan={plan} working={working} /> : null}
      {showGit ? <ChangesSection session={session} enabled={visible} /> : null}
      {showGit ? <GitSection repo={repo} /> : null}
      <ManageSection session={session} />
    </aside>
  )
}

function SectionHeader({ children }: { children: ReactNode }) {
  return <p className="text-[11px] font-medium tracking-wide text-ink-3 uppercase">{children}</p>
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

function PlanSection({ plan, working }: { plan: PlanSurface; working: boolean }) {
  const [open, setOpen] = useState(true)
  const entries = plan.entries ?? []
  const explanation = plan.explanation?.trim() ?? ''
  const states = entries.map(planStepState)
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
          {plan.title}
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
          {explanation ? (
            <div className="mt-2.5 text-[12px] leading-snug text-ink-2">
              <MessageMarkdown text={explanation} />
            </div>
          ) : null}
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
                        <PlanStepIcon state={state ?? 'pending'} active={working} />
                      </span>
                    ) : null}
                    <span className={`min-w-0 flex-1 ${state === 'completed' ? 'opacity-50' : ''}`}>
                      {entry.content}
                    </span>
                  </li>
                )
              })}
            </ul>
          ) : explanation ? null : (
            <p className="mt-2.5 text-[12px] italic text-ink-3">(no steps provided)</p>
          )}
        </>
      ) : null}
    </section>
  )
}

// How many file rows the panel shows before deferring to the review modal.
const CHANGES_PREVIEW_ROWS = 10

// What the session changed, at a glance: per-file +/− counts vs the
// worktree's fork point. The summary query is event-driven (invalidated at
// turn boundaries), runs only while this panel is mounted, and patch text is
// fetched per file inside the modal — never here.
function ChangesSection({ session, enabled }: { session: Session; enabled: boolean }) {
  // The opener owns the modal's expansion state: each open starts fresh with
  // the clicked file (or the first) expanded, with no reliance on the modal
  // remounting between opens.
  const [review, setReview] = useState<{ open: boolean; expanded: Record<string, boolean> }>({
    open: false,
    expanded: {},
  })
  const changes = useQuery({ ...sessionRepoChangesQuery(session.id), enabled })
  const data = changes.data
  if (!data || data.files.length === 0) return null
  const visible = data.files.slice(0, CHANGES_PREVIEW_ROWS)
  const hidden = data.files.length - visible.length
  const openReview = (key: string | null) => {
    const first = key ?? (data.files[0] ? fileKey(data.files[0]) : null)
    setReview({ open: true, expanded: first ? { [first]: true } : {} })
  }
  const toggleFile = (key: string) =>
    setReview((prev) => ({ ...prev, expanded: { ...prev.expanded, [key]: !prev.expanded[key] } }))
  return (
    <section className="flex flex-col gap-0.5">
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <SectionHeader>Changes</SectionHeader>
        <span className="shrink-0 font-mono text-[11px] tabular-nums">
          <span className="text-ok">+{data.total_added}</span> <span className="text-danger">−{data.total_deleted}</span>
        </span>
      </div>
      {visible.map((file) => (
        <button
          key={fileKey(file)}
          type="button"
          onClick={() => openReview(fileKey(file))}
          title={file.old_path ? `${file.old_path} → ${file.path}` : file.path}
          className="flex h-7 w-full cursor-pointer items-center gap-2 rounded-full px-2.5 text-left text-[13px] text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          <span
            className={`min-w-0 flex-1 truncate font-mono text-[12px] ${file.status === 'deleted' ? 'line-through opacity-60' : ''}`}
          >
            {basename(file.path)}
          </span>
          <FileCounts file={file} />
        </button>
      ))}
      {hidden > 0 ? (
        <button
          type="button"
          onClick={() => openReview(null)}
          className="h-7 w-full cursor-pointer rounded-full px-2.5 text-left text-[12px] text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
        >
          +{hidden} more {hidden === 1 ? 'file' : 'files'}
        </button>
      ) : null}
      <ActionRow icon={FileDiff} onClick={() => openReview(null)} hint="Opens every file's diff">
        Review changes
      </ActionRow>
      <DiffModal
        sessionId={session.id}
        changes={data}
        expanded={review.expanded}
        onToggle={toggleFile}
        open={review.open}
        onClose={() => setReview((prev) => ({ ...prev, open: false }))}
      />
    </section>
  )
}

function basename(path: string): string {
  const index = path.lastIndexOf('/')
  return index < 0 ? path : path.slice(index + 1)
}

function ManageSection({ session }: { session: Session }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const archive = useMutation({
    mutationFn: () => setSessionArchived(session.id, true),
    onSuccess: () => toast('Archived thread'),
    onError: (error: Error) => toast(`Couldn't archive: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(session.id) })
      queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
      queryClient.invalidateQueries({ queryKey: keys.allSessions })
      queryClient.invalidateQueries({ queryKey: keys.archivedSessions })
    },
  })

  return (
    <section className="flex flex-col gap-0.5">
      <div className="mb-1.5">
        <SectionHeader>Manage</SectionHeader>
      </div>
      <ActionRow
        icon={archive.isPending ? LoaderCircle : Archive}
        spin={archive.isPending}
        disabled={session.archived || archive.isPending}
        hint={session.archived ? 'Thread is archived' : 'Archives this thread and its children'}
        onClick={() => archive.mutate()}
      >
        {session.archived ? 'Archived' : 'Archive thread'}
      </ActionRow>
    </section>
  )
}

function GitSection({ repo }: { repo: ReturnType<typeof useRepoActions> }) {
  const { info, busy, web, branch, branchPath } = repo
  if (!info) return null
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
        <div className="flex h-7 items-center gap-2 px-2.5 text-[13px] text-ink-2">
          <GitBranch size={13} className="shrink-0 text-ink-3" />
          <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{branch || 'detached'}</span>
        </div>
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
        {info.is_worktree && info.main_branch && (info.dirty || !info.no_commits) ? (
          <ActionRow
            icon={busy === 'merge' ? LoaderCircle : ArrowLeftRight}
            spin={busy === 'merge'}
            disabled={busy !== null}
            hint={`Commits this session's work and merges its branch into ${info.main_branch}`}
            onClick={() => void repo.merge()}
          >
            {busy === 'merge' ? 'Merging…' : `Merge into ${info.main_branch}`}
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
