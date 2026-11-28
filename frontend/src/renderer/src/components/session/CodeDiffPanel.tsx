import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FolderGit2, LoaderCircle, RefreshCw } from 'lucide-react'
import { memo, useEffect, useState, type ReactNode } from 'react'
import {
  useCloudThreadRepos,
  useCloudThreadRepoChanges,
  useCloudThreadRepoDiff,
  type CloudThreadRepoChange,
  type CloudThreadRepoStatus,
} from '@/lib/api/cloudThreadRepos'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery, sessionRepoQuery } from '@/lib/api/sessions'
import { fileKey, type RepoChanges, type RepoFileChange, type RepoInfo } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

type DiffBodyTarget =
  | { kind: 'local'; sessionId: string; base?: string; file: RepoFileChange }
  | { kind: 'cloud'; sessionId: string; repo: string; path: string }

interface UnifiedChangedFile {
  key: string
  label: string
  path: string
  badge?: ReactNode
  meta?: ReactNode
  binary?: boolean
  target: DiffBodyTarget
}

interface UnifiedDiffSource {
  resetKey: string
  title: string
  subtitle: string
  subtitleTitle?: string
  summary?: string
  repos: string[]
  selectedRepo: string
  loadingLabel?: string
  error?: { message: string; retryLabel: string; onRetry: () => void }
  emptyMessage?: string
  files: UnifiedChangedFile[]
}

const STATUS_LABEL: Record<RepoFileChange['status'], string> = {
  added: 'added',
  untracked: 'new',
  deleted: 'deleted',
  renamed: 'renamed',
  modified: '',
}

function buildLocalDiffSource(
  sessionId: string,
  repo: RepoInfo | undefined,
  changes: { data?: RepoChanges; isPending: boolean; isError: boolean; error: Error | null; refetch: () => unknown },
): UnifiedDiffSource {
  const data = changes.isError ? undefined : changes.data
  const base = data?.base_kind === 'head' ? `HEAD ${shortRef(data.base || '')}`
    : data?.base_kind === 'fork' ? `Merge base ${shortRef(data.base || '')}`
      : data?.base_kind === 'empty' ? 'Empty tree'
        : data?.base ? `Base ${shortRef(data.base)}` : 'Base unknown'
  const title = `${repo?.is_worktree ? 'Worktree' : repo?.git ? 'Local checkout' : 'Checkout'}${repo?.branch ? ` · ${repo.branch}` : ''}`
  const summary = data
    ? `${data.files.length} ${data.files.length === 1 ? 'file' : 'files'} · +${data.total_added} −${data.total_deleted}`
    : 'Code diff'
  return {
    resetKey: sessionId,
    title,
    subtitle: `${base} → working tree`,
    subtitleTitle: data?.base,
    summary,
    repos: [],
    selectedRepo: '',
    loadingLabel: changes.isPending ? 'Loading changes…' : undefined,
    error: changes.isError && changes.error ? {
      message: `Couldn’t check code changes: ${changes.error.message}`,
      retryLabel: 'Retry code changes',
      onRetry: () => void changes.refetch(),
    } : undefined,
    emptyMessage: data && data.files.length === 0
      ? (data.base_kind === 'head' ? 'No uncommitted changes in this checkout.' : 'No changes against this base in this checkout.')
      : undefined,
    files: (data?.files ?? []).map((file) => {
      const status = STATUS_LABEL[file.status]
      return {
        key: fileKey(file),
        label: `${file.old_path ? `${file.old_path} → ` : ''}${file.path}`,
        path: file.path,
        badge: status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null,
        meta: <FileCounts file={file} />,
        binary: file.binary,
        target: { kind: 'local', sessionId, base: data?.base, file },
      }
    }),
  }
}

function buildCloudDiffSource(
  sessionId: string,
  repos: CloudThreadRepoStatus[],
  activeRepo: CloudThreadRepoStatus | undefined,
  status: { isPending: boolean; isError: boolean; error: Error | null; refetch: () => unknown },
  changes: { data?: ReturnType<typeof useCloudThreadRepoChanges>['data']; isPending: boolean; isError: boolean; error: Error | null; refetch: () => unknown },
): UnifiedDiffSource {
  const selector = activeRepo?.repo ?? ''
  const data = changes.isError ? undefined : changes.data
  const baseBranch = data?.base_branch || activeRepo?.base_branch || ''
  const branch = data?.branch || activeRepo?.branch || baseBranch
  const added = data?.changes.reduce((acc, item) => acc + (item.additions ?? 0), 0) ?? 0
  const deleted = data?.changes.reduce((acc, item) => acc + (item.deletions ?? 0), 0) ?? 0
  const fileLabel = data ? `${data.changes.length} ${data.changes.length === 1 ? 'file' : 'files'}` : ''
  const summary = !activeRepo
    ? undefined
    : !data
      ? 'Code diff'
      : added || deleted
        ? `${fileLabel} · +${added} −${deleted}`
        : fileLabel

  return {
    resetKey: JSON.stringify([sessionId, selector]),
    title: data?.repo || activeRepo?.repo || 'Cloud repositories',
    subtitle: activeRepo ? [baseBranch, branch].filter(Boolean).join(' → ') : 'Code changes',
    summary,
    repos: repos.map((item) => item.repo),
    selectedRepo: selector,
    loadingLabel: status.isPending
      ? 'Loading repositories…'
      : activeRepo && changes.isPending
        ? 'Loading changes…'
        : undefined,
    error: status.isError && status.error
      ? {
          message: `Couldn’t load repository status: ${status.error.message}`,
          retryLabel: 'Retry',
          onRetry: () => void status.refetch(),
        }
      : activeRepo && changes.isError && changes.error
        ? {
            message: `Couldn’t load code changes: ${changes.error.message}`,
            retryLabel: 'Retry code changes',
            onRetry: () => void changes.refetch(),
          }
        : undefined,
    emptyMessage: !status.isPending && !status.isError && !activeRepo
      ? 'No repository is attached to this Cloud thread. Attach one in Overview or when starting a new thread.'
      : data && data.changes.length === 0
        ? `No changes against ${baseBranch} yet.`
        : undefined,
    files: (data?.changes ?? []).map((change) => ({
      key: change.path,
      label: change.path,
      path: change.path,
      badge: cloudChangeBadge(change),
      meta: cloudChangeMeta(change),
      target: { kind: 'cloud', sessionId, repo: selector, path: change.path },
    })),
  }
}

function useCodeDiffSource(
  sessionId: string,
  visible: boolean,
  isCloud: boolean,
  selectedRepo?: string,
): UnifiedDiffSource {
  const localChanges = useQuery({ ...sessionRepoChangesQuery(sessionId), enabled: visible && !isCloud })
  const localRepo = useQuery({ ...sessionRepoQuery(sessionId), enabled: visible && !isCloud })

  const cloudStatus = useCloudThreadRepos(sessionId, visible && isCloud)
  const cloudRepos = cloudStatus.data ?? []
  const activeCloudRepo = cloudRepos.find((item) => item.repo === selectedRepo) ?? cloudRepos[0]
  const cloudChanges = useCloudThreadRepoChanges(
    sessionId,
    activeCloudRepo?.repo ?? '',
    visible && isCloud && Boolean(activeCloudRepo),
  )

  return isCloud
    ? buildCloudDiffSource(sessionId, cloudRepos, activeCloudRepo, cloudStatus, cloudChanges)
    : buildLocalDiffSource(sessionId, localRepo.data, localChanges)
}

export const CodeDiffPanel = memo(function CodeDiffPanel({
  sessionId,
  visible,
  isCloud = false,
  repo,
  onSelectRepo,
}: {
  sessionId: string
  visible: boolean
  isCloud?: boolean
  repo?: string
  onSelectRepo: (repo: string) => void
}) {
  const source = useCodeDiffSource(sessionId, visible, isCloud, repo)
  const { expanded, toggle } = useAutoExpandFirst(source.resetKey, source.files[0]?.key ?? '')

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <FolderGit2 size={15} className="shrink-0 text-ink-3" aria-hidden />
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="truncate text-[12px] text-ink-2">{source.title}</p>
          <p className="truncate text-[11px] text-ink-3" title={source.subtitleTitle}>{source.subtitle}</p>
        </div>
        {source.summary ? (
          <span className="hidden shrink-0 font-mono text-[11px] text-ink-3 tabular-nums sm:inline">{source.summary}</span>
        ) : null}
      </div>
      {source.repos.length > 1 ? (
        <div role="group" aria-label="Filter changes by repository" className="flex shrink-0 flex-wrap gap-1 border-b border-border px-3 py-2">
          {source.repos.map((name) => (
            <button
              key={name}
              type="button"
              aria-pressed={name === source.selectedRepo}
              onClick={() => onSelectRepo(name)}
              className="min-h-8 min-w-0 max-w-full cursor-pointer truncate rounded-full bg-surface px-2.5 text-[12px] text-ink-2 hover:bg-surface-2 aria-pressed:bg-primary-soft aria-pressed:text-primary"
              title={name}
            >
              {name}
            </button>
          ))}
        </div>
      ) : null}
      <div className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto">
        {source.error ? (
          <div className="space-y-2 px-3 py-4 text-[12px] text-danger">
            <p>{source.error.message}</p>
            <button type="button" onClick={source.error.onRetry} className="inline-flex cursor-pointer items-center gap-1.5 hover:underline">
              <RefreshCw size={12} aria-hidden />
              {source.error.retryLabel}
            </button>
          </div>
        ) : source.loadingLabel ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            {source.loadingLabel}
          </div>
        ) : source.emptyMessage ? (
          <div className="px-3 py-4 text-[12px] text-ink-3">{source.emptyMessage}</div>
        ) : (
          source.files.map((file) => {
            const open = Boolean(expanded[file.key])
            return (
              <section key={file.key} className="border-t border-border first:border-t-0">
                <button
                  type="button"
                  aria-expanded={open}
                  onClick={() => toggle(file.key)}
                  className={`flex min-h-10 w-full cursor-pointer items-center gap-2 px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2 ${
                    open ? 'bg-surface-2/70' : 'bg-surface'
                  }`}
                >
                  <ChevronDown
                    size={13}
                    className={`shrink-0 text-ink-3 transition-transform duration-200 ease-out ${open ? '' : '-rotate-90'}`}
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink-2" title={file.path}>
                    {file.label}
                  </span>
                  {file.badge}
                  {file.meta}
                </button>
                {open ? (
                  file.binary ? (
                    <div className="border-t border-border">
                      <DiffView patch="" path={file.path} binary />
                    </div>
                  ) : file.target.kind === 'local' ? (
                    <LocalFileDiffBody target={file.target} visible={visible} />
                  ) : (
                    <CloudFileDiffBody target={file.target} visible={visible} />
                  )
                ) : null}
              </section>
            )
          })
        )}
      </div>
    </div>
  )
})

function useAutoExpandFirst(resetKey: string, firstKey: string) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  useEffect(() => setExpanded({}), [resetKey])
  useEffect(() => {
    if (!firstKey) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstKey]: true }))
  }, [firstKey])
  const toggle = (key: string) => setExpanded((current) => ({ ...current, [key]: !current[key] }))
  return { expanded, toggle }
}

function DiffBodyFrame({ pending, error, children }: { pending: boolean; error: Error | null; children: ReactNode }) {
  if (pending) {
    return (
      <div className="flex items-center gap-2 border-t border-border px-3 py-3 text-[12px] text-ink-3">
        <LoaderCircle size={13} className="animate-spin" aria-hidden />
        Loading diff…
      </div>
    )
  }
  if (error) {
    return (
      <p className="border-t border-border px-3 py-3 text-[12px] text-danger">
        Couldn&apos;t load the diff: {error.message}
      </p>
    )
  }
  return <div className="border-t border-border">{children}</div>
}

function LocalFileDiffBody({
  target,
  visible,
}: {
  target: Extract<DiffBodyTarget, { kind: 'local' }>
  visible: boolean
}) {
  const diff = useQuery({ ...sessionRepoFileDiffQuery(target.sessionId, target.file, target.base), enabled: visible })
  return (
    <DiffBodyFrame pending={diff.isPending} error={diff.error}>
      {diff.data ? (
        <DiffView
          patch={diff.data.patch}
          path={diff.data.path || target.file.path}
          binary={diff.data.binary}
          truncated={diff.data.truncated}
        />
      ) : null}
    </DiffBodyFrame>
  )
}

function CloudFileDiffBody({
  target,
  visible,
}: {
  target: Extract<DiffBodyTarget, { kind: 'cloud' }>
  visible: boolean
}) {
  const diff = useCloudThreadRepoDiff(target.sessionId, target.repo, target.path, visible)
  const entries = diff.data?.diffs ?? []
  return (
    <DiffBodyFrame pending={diff.isPending} error={diff.error}>
      {entries.length === 0 ? (
        <p className="px-3 py-3 text-[12px] italic text-ink-3">No diff preview available.</p>
      ) : (
        entries.map((entry, idx) => (
          <div key={`${entry.source}:${entry.path}:${idx}`} className={idx > 0 ? 'border-t border-border' : ''}>
            {entry.patch ? (
              <DiffView patch={entry.patch} path={entry.path || target.path} />
            ) : entry.content !== undefined && entry.content !== '' ? (
              <div className="overflow-x-auto bg-bg/45 px-3 py-2.5 font-mono text-[12px] leading-[1.55] text-ink-2 select-text">
                <pre className="whitespace-pre">{entry.content}</pre>
              </div>
            ) : (
              <p className="px-3 py-3 text-[12px] italic text-ink-3">
                {entry.status === 'workspace_deleted'
                  ? 'Deleted in the workspace.'
                  : entry.source === 'workspace'
                    ? 'Workspace file preview unavailable (binary or exceeds 128 KiB).'
                    : 'No patch available for this file.'}
              </p>
            )}
          </div>
        ))
      )}
    </DiffBodyFrame>
  )
}

function cloudChangeBadge(change: CloudThreadRepoChange): ReactNode {
  if (change.source === 'workspace') {
    return <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">workspace</span>
  }
  if (change.status && change.status !== 'modified') {
    return <span className="shrink-0 text-[11px] text-ink-3">{change.status}</span>
  }
  return null
}

function cloudChangeMeta(change: CloudThreadRepoChange): ReactNode {
  if (change.additions || change.deletions) {
    return (
      <span className="shrink-0 font-mono text-[11px] tabular-nums">
        {change.additions ? <span className="text-ok">+{change.additions}</span> : null}
        {change.additions && change.deletions ? ' ' : null}
        {change.deletions ? <span className="text-danger">−{change.deletions}</span> : null}
      </span>
    )
  }
  if (change.size) {
    return <span className="shrink-0 font-mono text-[11px] text-ink-3 tabular-nums">{formatBytes(change.size)}</span>
  }
  return null
}

function shortRef(value: string): string {
  return value.length > 18 && /^[a-f0-9]+$/i.test(value) ? value.slice(0, 7) : value
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
