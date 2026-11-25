import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FolderGit2, LoaderCircle, RefreshCw } from 'lucide-react'
import { memo, useEffect, useMemo, useState } from 'react'
import {
  useCloudThreadRepo,
  useCloudThreadRepoChanges,
  useCloudThreadRepoDiff,
  type CloudThreadRepoChange,
} from '@/lib/api/cloudGitHub'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery, sessionRepoQuery } from '@/lib/api/sessions'
import { fileKey, type RepoFileChange } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

export const CodeDiffPanel = memo(function CodeDiffPanel({
  sessionId,
  visible,
  isCloud = false,
}: {
  sessionId: string
  visible: boolean
  isCloud?: boolean
}) {
  if (isCloud) {
    return <CloudCodeDiffPanel sessionId={sessionId} visible={visible} />
  }
  return <LocalCodeDiffPanel sessionId={sessionId} visible={visible} />
})

function LocalCodeDiffPanel({ sessionId, visible }: { sessionId: string; visible: boolean }) {
  const changes = useQuery({ ...sessionRepoChangesQuery(sessionId), enabled: visible })
  const repo = useQuery({ ...sessionRepoQuery(sessionId), enabled: visible })
  const data = changes.isError ? undefined : changes.data
  const firstKey = data?.files[0] ? fileKey(data.files[0]) : ''
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  useEffect(() => setExpanded({}), [sessionId])
  useEffect(() => {
    if (!firstKey) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstKey]: true }))
  }, [firstKey])

  const summary = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.files.length} ${data.files.length === 1 ? 'file' : 'files'}`
    return `${files} · +${data.total_added} −${data.total_deleted}`
  }, [data])
  const base = data?.base_kind === 'head' ? `HEAD ${shortRef(data.base || '')}`
    : data?.base_kind === 'fork' ? `Merge base ${shortRef(data.base || '')}`
      : data?.base_kind === 'empty' ? 'Empty tree'
        : data?.base ? `Base ${shortRef(data.base)}` : 'Base unknown'

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <FolderGit2 size={15} className="shrink-0 text-ink-3" aria-hidden />
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="truncate text-[12px] text-ink-2">{repo.data?.is_worktree ? 'Worktree' : repo.data?.git ? 'Local checkout' : 'Checkout'}{repo.data?.branch ? ` · ${repo.data.branch}` : ''}</p>
          <p className="truncate text-[11px] text-ink-3" title={data?.base}>{base} → working tree</p>
        </div>
        <span className="hidden shrink-0 font-mono text-[11px] text-ink-3 tabular-nums sm:inline">{summary}</span>
      </div>
      <div className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto">
        {changes.isError ? (
          <div className="space-y-2 px-3 py-4 text-[12px] text-danger">
            <p>Couldn’t check code changes: {changes.error.message}</p>
            <button type="button" onClick={() => void changes.refetch()} className="inline-flex cursor-pointer items-center gap-1.5 hover:underline"><RefreshCw size={12} aria-hidden />Retry code changes</button>
          </div>
        ) : null}
        {changes.isPending ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading changes…
          </div>
        ) : null}
        {data && data.files.length === 0 ? (
          <div className="px-3 py-4 text-[12px] text-ink-3">{data.base_kind === 'head' ? 'No uncommitted changes in this checkout.' : 'No changes against this base in this checkout.'}</div>
        ) : null}
        {data?.files.length
          ? data.files.map((file) => {
              const key = fileKey(file)
              return (
                <DiffFileSection
                  key={key}
                  sessionId={sessionId}
                  base={data.base}
                  file={file}
                  expanded={Boolean(expanded[key])}
                  visible={visible}
                  onToggle={() =>
                    setExpanded((current) => ({ ...current, [key]: !current[key] }))
                  }
                />
              )
            })
          : null}
      </div>
    </div>
  )
}

function CloudCodeDiffPanel({ sessionId, visible }: { sessionId: string; visible: boolean }) {
  const repo = useCloudThreadRepo(sessionId, visible)
  const attached = Boolean(repo.data?.attached && repo.data?.repo)
  const changes = useCloudThreadRepoChanges(sessionId, visible && attached)
  const data = changes.isError ? undefined : changes.data
  const firstPath = data?.changes[0]?.path ?? ''
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  useEffect(() => setExpanded({}), [sessionId])
  useEffect(() => {
    if (!firstPath) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstPath]: true }))
  }, [firstPath])

  const summary = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.changes.length} ${data.changes.length === 1 ? 'file' : 'files'}`
    const added = data.changes.reduce((acc, item) => acc + (item.additions ?? 0), 0)
    const deleted = data.changes.reduce((acc, item) => acc + (item.deletions ?? 0), 0)
    return added || deleted ? `${files} · +${added} −${deleted}` : files
  }, [data])

  const repoTitle = data?.repo || repo.data?.repo || 'Cloud repository'
  const baseBranch = data?.base_branch || repo.data?.base_branch || 'main'
  const branch = data?.branch || repo.data?.branch || baseBranch

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-11 shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <FolderGit2 size={15} className="shrink-0 text-ink-3" aria-hidden />
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="truncate text-[12px] text-ink-2">
            {repoTitle}
            {attached && branch ? ` · ${branch}` : ''}
          </p>
          <p className="truncate text-[11px] text-ink-3">
            {attached ? `${baseBranch} → ${branch}` : 'No repository attached'}
          </p>
        </div>
        {attached ? (
          <span className="hidden shrink-0 font-mono text-[11px] text-ink-3 tabular-nums sm:inline">{summary}</span>
        ) : null}
      </div>
      <div className="scrollbar-quiet min-h-0 flex-1 overflow-y-auto">
        {repo.isPending ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading repository…
          </div>
        ) : repo.isError ? (
          <div className="space-y-2 px-3 py-4 text-[12px] text-danger">
            <p>Couldn’t load repository status: {repo.error.message}</p>
            <button type="button" onClick={() => void repo.refetch()} className="inline-flex cursor-pointer items-center gap-1.5 hover:underline">
              <RefreshCw size={12} aria-hidden />
              Retry
            </button>
          </div>
        ) : !attached ? (
          <div className="px-3 py-4 text-[12px] text-ink-3">
            No repository is attached to this Cloud thread. Attach one in Overview or when starting a new thread.
          </div>
        ) : changes.isError ? (
          <div className="space-y-2 px-3 py-4 text-[12px] text-danger">
            <p>Couldn’t load code changes: {changes.error.message}</p>
            <button type="button" onClick={() => void changes.refetch()} className="inline-flex cursor-pointer items-center gap-1.5 hover:underline">
              <RefreshCw size={12} aria-hidden />
              Retry code changes
            </button>
          </div>
        ) : changes.isPending ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading changes…
          </div>
        ) : data && data.changes.length === 0 ? (
          <div className="px-3 py-4 text-[12px] text-ink-3">No changes against {baseBranch} yet.</div>
        ) : (
          data?.changes.map((change) => (
            <CloudDiffFileSection
              key={`${change.source}:${change.path}`}
              sessionId={sessionId}
              change={change}
              expanded={Boolean(expanded[change.path])}
              visible={visible}
              onToggle={() =>
                setExpanded((current) => ({ ...current, [change.path]: !current[change.path] }))
              }
            />
          ))
        )}
      </div>
    </div>
  )
}

function CloudDiffFileSection({
  sessionId,
  change,
  expanded,
  visible,
  onToggle,
}: {
  sessionId: string
  change: CloudThreadRepoChange
  expanded: boolean
  visible: boolean
  onToggle: () => void
}) {
  const hasCounts = Boolean(change.additions || change.deletions)
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
        <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink-2" title={change.path}>
          {change.path}
        </span>
        {change.source === 'workspace' ? (
          <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-[10.5px] text-ink-3">workspace</span>
        ) : change.status && change.status !== 'modified' ? (
          <span className="shrink-0 text-[11px] text-ink-3">{change.status}</span>
        ) : null}
        {hasCounts ? (
          <span className="shrink-0 font-mono text-[11px] tabular-nums">
            {change.additions ? <span className="text-ok">+{change.additions}</span> : null}
            {change.additions && change.deletions ? ' ' : null}
            {change.deletions ? <span className="text-danger">−{change.deletions}</span> : null}
          </span>
        ) : change.size ? (
          <span className="shrink-0 font-mono text-[11px] text-ink-3 tabular-nums">{formatBytes(change.size)}</span>
        ) : null}
      </button>
      {expanded ? <CloudFileDiffBody sessionId={sessionId} path={change.path} visible={visible} /> : null}
    </section>
  )
}

function CloudFileDiffBody({
  sessionId,
  path,
  visible,
}: {
  sessionId: string
  path: string
  visible: boolean
}) {
  const diff = useCloudThreadRepoDiff(sessionId, path, visible)
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
              {entry.source === 'workspace'
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

function DiffFileSection({
  sessionId,
  base,
  file,
  expanded,
  visible,
  onToggle,
}: {
  sessionId: string
  base?: string
  file: RepoFileChange
  expanded: boolean
  visible: boolean
  onToggle: () => void
}) {
  const status = STATUS_LABEL[file.status]
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
        <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink-2" title={file.path}>
          {file.old_path ? `${file.old_path} → ` : ''}
          {file.path}
        </span>
        {status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null}
        <FileCounts file={file} />
      </button>
      {expanded && file.binary ? (
        <div className="border-t border-border">
          <DiffView patch="" path={file.path} binary />
        </div>
      ) : null}
      {expanded && !file.binary ? <FileDiffBody sessionId={sessionId} base={base} file={file} visible={visible} /> : null}
    </section>
  )
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
