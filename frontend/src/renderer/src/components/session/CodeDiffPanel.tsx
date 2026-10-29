import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FolderGit2, LoaderCircle, RefreshCw } from 'lucide-react'
import { memo, useEffect, useMemo, useState } from 'react'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery, sessionRepoQuery } from '@/lib/api/sessions'
import { fileKey, type RepoFileChange } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

export const CodeDiffPanel = memo(function CodeDiffPanel({
  sessionId,
  visible,
}: {
  sessionId: string
  visible: boolean
}) {
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
})

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
