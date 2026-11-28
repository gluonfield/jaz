import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FolderGit2, LoaderCircle, RefreshCw } from 'lucide-react'
import { memo, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  useCloudThreadRepos,
  useCloudThreadRepoChanges,
  useCloudThreadRepoDiff,
  type CloudThreadRepoChange,
} from '@/lib/api/cloudThreadRepos'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery, sessionRepoQuery } from '@/lib/api/sessions'
import { fileKey, type RepoFileChange } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

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
  return isCloud
    ? <CloudCodeDiffPanel sessionId={sessionId} visible={visible} selectedRepo={repo} onSelectRepo={onSelectRepo} />
    : <LocalCodeDiffPanel sessionId={sessionId} visible={visible} />
})

function DiffPanelShell({
  title,
  subtitle,
  subtitleTitle,
  summary,
  filters,
  children,
}: {
  title: string
  subtitle: string
  subtitleTitle?: string
  summary?: string
  filters?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <FolderGit2 size={15} className="shrink-0 text-ink-3" aria-hidden />
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="truncate text-[12px] text-ink-2">{title}</p>
          <p className="truncate text-[11px] text-ink-3" title={subtitleTitle}>{subtitle}</p>
        </div>
        {summary ? (
          <span className="hidden shrink-0 font-mono text-[11px] text-ink-3 tabular-nums sm:inline">{summary}</span>
        ) : null}
      </div>
      {filters}
      <div className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto">{children}</div>
    </div>
  )
}

function CollapsibleDiffSection({
  label,
  title,
  badge,
  meta,
  expanded,
  onToggle,
  children,
}: {
  label: string
  title: string
  badge?: ReactNode
  meta?: ReactNode
  expanded: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section className="border-t border-border first:border-t-0">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={onToggle}
        className={`flex min-h-10 w-full cursor-pointer items-center gap-2 px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2 ${
          expanded ? 'bg-surface-2/70' : 'bg-surface'
        }`}
      >
        <ChevronDown
          size={13}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ease-out ${expanded ? '' : '-rotate-90'}`}
          aria-hidden
        />
        <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink-2" title={title}>
          {label}
        </span>
        {badge}
        {meta}
      </button>
      {expanded ? children : null}
    </section>
  )
}

function DiffLoadingRow({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
      <LoaderCircle size={13} className="animate-spin" aria-hidden />
      {label}
    </div>
  )
}

function DiffErrorBlock({
  message,
  retryLabel,
  onRetry,
}: {
  message: string
  retryLabel: string
  onRetry: () => void
}) {
  return (
    <div className="space-y-2 px-3 py-4 text-[12px] text-danger">
      <p>{message}</p>
      <button type="button" onClick={onRetry} className="inline-flex cursor-pointer items-center gap-1.5 hover:underline">
        <RefreshCw size={12} aria-hidden />
        {retryLabel}
      </button>
    </div>
  )
}

function useAutoExpandFirst(sessionId: string, firstKey: string) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  useEffect(() => setExpanded({}), [sessionId])
  useEffect(() => {
    if (!firstKey) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstKey]: true }))
  }, [firstKey])
  const toggle = (key: string) => setExpanded((current) => ({ ...current, [key]: !current[key] }))
  return { expanded, toggle }
}

function LocalCodeDiffPanel({ sessionId, visible }: { sessionId: string; visible: boolean }) {
  const changes = useQuery({ ...sessionRepoChangesQuery(sessionId), enabled: visible })
  const repo = useQuery({ ...sessionRepoQuery(sessionId), enabled: visible })
  const data = changes.isError ? undefined : changes.data
  const { expanded, toggle } = useAutoExpandFirst(sessionId, data?.files[0] ? fileKey(data.files[0]) : '')

  const summary = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.files.length} ${data.files.length === 1 ? 'file' : 'files'}`
    return `${files} · +${data.total_added} −${data.total_deleted}`
  }, [data])
  const base = data?.base_kind === 'head' ? `HEAD ${shortRef(data.base || '')}`
    : data?.base_kind === 'fork' ? `Merge base ${shortRef(data.base || '')}`
      : data?.base_kind === 'empty' ? 'Empty tree'
        : data?.base ? `Base ${shortRef(data.base)}` : 'Base unknown'
  const title = `${repo.data?.is_worktree ? 'Worktree' : repo.data?.git ? 'Local checkout' : 'Checkout'}${repo.data?.branch ? ` · ${repo.data.branch}` : ''}`

  return (
    <DiffPanelShell title={title} subtitle={`${base} → working tree`} subtitleTitle={data?.base} summary={summary}>
      {changes.isError ? (
        <DiffErrorBlock
          message={`Couldn’t check code changes: ${changes.error.message}`}
          retryLabel="Retry code changes"
          onRetry={() => void changes.refetch()}
        />
      ) : changes.isPending ? (
        <DiffLoadingRow label="Loading changes…" />
      ) : data && data.files.length === 0 ? (
        <div className="px-3 py-4 text-[12px] text-ink-3">
          {data.base_kind === 'head' ? 'No uncommitted changes in this checkout.' : 'No changes against this base in this checkout.'}
        </div>
      ) : (
        data?.files.map((file) => {
          const key = fileKey(file)
          const status = STATUS_LABEL[file.status]
          return (
            <CollapsibleDiffSection
              key={key}
              label={`${file.old_path ? `${file.old_path} → ` : ''}${file.path}`}
              title={file.path}
              badge={status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null}
              meta={<FileCounts file={file} />}
              expanded={Boolean(expanded[key])}
              onToggle={() => toggle(key)}
            >
              {file.binary ? (
                <div className="border-t border-border">
                  <DiffView patch="" path={file.path} binary />
                </div>
              ) : (
                <FileDiffBody sessionId={sessionId} base={data.base} file={file} visible={visible} />
              )}
            </CollapsibleDiffSection>
          )
        })
      )}
    </DiffPanelShell>
  )
}

function CloudCodeDiffPanel({ sessionId, visible, selectedRepo, onSelectRepo }: {
  sessionId: string
  visible: boolean
  selectedRepo?: string
  onSelectRepo: (repo: string) => void
}) {
  const status = useCloudThreadRepos(sessionId, visible)
  const repos = status.data ?? []
  const repo = repos.find((item) => item.repo === selectedRepo) ?? repos[0]
  const selector = repo?.repo ?? ''
  const changes = useCloudThreadRepoChanges(sessionId, selector, visible && Boolean(repo))
  const data = changes.isError ? undefined : changes.data

  const summary = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.changes.length} ${data.changes.length === 1 ? 'file' : 'files'}`
    const added = data.changes.reduce((acc, item) => acc + (item.additions ?? 0), 0)
    const deleted = data.changes.reduce((acc, item) => acc + (item.deletions ?? 0), 0)
    return added || deleted ? `${files} · +${added} −${deleted}` : files
  }, [data])

  const repoTitle = data?.repo || repo?.repo || 'Cloud repositories'
  const baseBranch = data?.base_branch || repo?.base_branch || ''
  const branch = data?.branch || repo?.branch || baseBranch

  return (
    <DiffPanelShell
      title={repoTitle}
      subtitle={repo ? [baseBranch, branch].filter(Boolean).join(' → ') : 'Code changes'}
      summary={repo ? summary : undefined}
      filters={repos.length > 1 ? (
        <div role="group" aria-label="Filter changes by repository" className="flex shrink-0 flex-wrap gap-1 border-b border-border px-3 py-2">
          {repos.map(({ repo: name }) => (
            <button
              key={name}
              type="button"
              aria-pressed={name === selector}
              onClick={() => onSelectRepo(name)}
              className="min-h-8 min-w-0 max-w-full cursor-pointer truncate rounded-full bg-surface px-2.5 text-[12px] text-ink-2 hover:bg-surface-2 aria-pressed:bg-primary-soft aria-pressed:text-primary"
              title={name}
            >
              {name}
            </button>
          ))}
        </div>
      ) : undefined}
    >
      {status.isPending ? (
        <DiffLoadingRow label="Loading repositories…" />
      ) : status.isError ? (
        <DiffErrorBlock
          message={`Couldn’t load repository status: ${status.error.message}`}
          retryLabel="Retry"
          onRetry={() => void status.refetch()}
        />
      ) : !repo ? (
        <div className="px-3 py-4 text-[12px] text-ink-3">
          No repository is attached to this Cloud thread. Attach one in Overview or when starting a new thread.
        </div>
      ) : changes.isError ? (
        <DiffErrorBlock
          message={`Couldn’t load code changes: ${changes.error.message}`}
          retryLabel="Retry code changes"
          onRetry={() => void changes.refetch()}
        />
      ) : changes.isPending ? (
        <DiffLoadingRow label="Loading changes…" />
      ) : data && data.changes.length === 0 ? (
        <div className="px-3 py-4 text-[12px] text-ink-3">No changes against {baseBranch} yet.</div>
      ) : (
        <CloudChangedFiles key={JSON.stringify([sessionId, selector])} sessionId={sessionId} repo={selector} changes={data?.changes ?? []} visible={visible} />
      )}
    </DiffPanelShell>
  )
}

function CloudChangedFiles({ sessionId, repo, changes, visible }: {
  sessionId: string
  repo: string
  changes: CloudThreadRepoChange[]
  visible: boolean
}) {
  const { expanded, toggle } = useAutoExpandFirst(sessionId, changes[0]?.path ?? '')
  return changes.map((change) => (
    <CollapsibleDiffSection
      key={change.path}
      label={change.path}
      title={change.path}
      badge={cloudChangeBadge(change)}
      meta={cloudChangeMeta(change)}
      expanded={Boolean(expanded[change.path])}
      onToggle={() => toggle(change.path)}
    >
      <CloudFileDiffBody sessionId={sessionId} repo={repo} path={change.path} visible={visible} />
    </CollapsibleDiffSection>
  ))
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

function CloudFileDiffBody({ sessionId, repo, path, visible }: {
  sessionId: string
  repo: string
  path: string
  visible: boolean
}) {
  const diff = useCloudThreadRepoDiff(sessionId, repo, path, visible)
  if (diff.isPending) {
    return (
      <div className="flex items-center gap-2 border-t border-border px-3 py-3 text-[12px] text-ink-3">
        <LoaderCircle size={13} className="animate-spin" aria-hidden />
        Loading diff…
      </div>
    )
  }
  if (diff.isError) {
    return (
      <p className="border-t border-border px-3 py-3 text-[12px] text-danger">
        Couldn&apos;t load the diff: {(diff.error as Error).message}
      </p>
    )
  }
  const entries = diff.data?.diffs ?? []
  if (entries.length === 0) {
    return <p className="border-t border-border px-3 py-3 text-[12px] italic text-ink-3">No diff preview available.</p>
  }
  return (
    <div className="border-t border-border">
      {entries.map((entry, idx) => (
        <div key={`${entry.source}:${entry.path}:${idx}`} className={idx > 0 ? 'border-t border-border' : ''}>
          {entry.patch ? (
            <DiffView patch={entry.patch} path={entry.path || path} />
          ) : entry.content !== undefined && entry.content !== '' ? (
            <div className="overflow-x-auto bg-bg/45 px-3 py-2.5 font-mono text-[12px] leading-[1.55] text-ink-2 select-text">
              <pre className="whitespace-pre">{entry.content}</pre>
            </div>
          ) : (
            <p className="px-3 py-3 text-[12px] italic text-ink-3">
              {entry.status === 'workspace_deleted' ? 'Deleted in the workspace.' : entry.source === 'workspace'
                ? 'Workspace file preview unavailable (binary or exceeds 128 KiB).'
                : 'No patch available for this file.'}
            </p>
          )}
        </div>
      ))}
    </div>
  )
}

const STATUS_LABEL: Record<RepoFileChange['status'], string> = {
  added: 'added',
  untracked: 'new',
  deleted: 'deleted',
  renamed: 'renamed',
  modified: '',
}

function FileDiffBody({
  sessionId,
  base,
  file,
  visible,
}: {
  sessionId: string
  base?: string
  file: RepoFileChange
  visible: boolean
}) {
  const diff = useQuery({ ...sessionRepoFileDiffQuery(sessionId, file, base), enabled: visible })
  if (diff.isPending) {
    return (
      <div className="flex items-center gap-2 border-t border-border px-3 py-3 text-[12px] text-ink-3">
        <LoaderCircle size={13} className="animate-spin" aria-hidden />
        Loading diff…
      </div>
    )
  }
  if (diff.isError) {
    return (
      <p className="border-t border-border px-3 py-3 text-[12px] text-danger">
        Couldn&apos;t load the diff: {(diff.error as Error).message}
      </p>
    )
  }
  return (
    <div className="border-t border-border">
      <DiffView
        patch={diff.data.patch}
        path={diff.data.path || file.path}
        binary={diff.data.binary}
        truncated={diff.data.truncated}
      />
    </div>
  )
}

function shortRef(value: string): string {
  return value.length > 18 && /^[a-f0-9]+$/i.test(value) ? value.slice(0, 7) : value
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
