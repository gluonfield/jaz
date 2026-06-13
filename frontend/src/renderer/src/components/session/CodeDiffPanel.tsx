import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FileDiff, LoaderCircle } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery } from '@/lib/api/sessions'
import { fileKey, type RepoFileChange, type Session } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

export const CODE_DIFF_PANEL_WIDTH = 640

export function CodeDiffPanel({ session, visible }: { session: Session; visible: boolean }) {
  const changes = useQuery({ ...sessionRepoChangesQuery(session.id), enabled: visible })
  const data = changes.data
  const firstKey = data?.files[0] ? fileKey(data.files[0]) : ''
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  useEffect(() => setExpanded({}), [session.id])
  useEffect(() => {
    if (!firstKey) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstKey]: true }))
  }, [firstKey])

  const title = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.files.length} ${data.files.length === 1 ? 'file' : 'files'}`
    return `${files} · +${data.total_added} −${data.total_deleted}`
  }, [data])

  return (
    <aside
      style={{ width: CODE_DIFF_PANEL_WIDTH }}
      className="flex h-full shrink-0 flex-col border-l border-border bg-bg"
    >
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
        <FileDiff size={15} className="shrink-0 text-ink-3" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium text-ink">Code Diff</p>
          <p className="truncate font-mono text-[11px] text-ink-3 tabular-nums">{title}</p>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-3">
        {changes.isPending ? (
          <div className="flex items-center gap-2 px-2 py-3 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading changes…
          </div>
        ) : null}
        {data && data.files.length === 0 ? (
          <div className="rounded-card bg-surface px-3 py-3 text-[12px] text-ink-3">
            No code changes to show.
          </div>
        ) : null}
        {data?.files.length ? (
          <div className="flex flex-col gap-2">
            {data.files.map((file) => {
              const key = fileKey(file)
              return (
                <DiffFileSection
                  key={key}
                  sessionId={session.id}
                  base={data.base}
                  file={file}
                  expanded={Boolean(expanded[key])}
                  onToggle={() =>
                    setExpanded((current) => ({ ...current, [key]: !current[key] }))
                  }
                />
              )
            })}
          </div>
        ) : null}
      </div>
    </aside>
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
  onToggle,
}: {
  sessionId: string
  base?: string
  file: RepoFileChange
  expanded: boolean
  onToggle: () => void
}) {
  const status = STATUS_LABEL[file.status]
  return (
    <section className="overflow-hidden rounded-card bg-surface ring-1 ring-border">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={onToggle}
        className="flex min-h-10 w-full cursor-pointer items-center gap-2 px-3 py-2 text-left transition-colors duration-150 hover:bg-surface-2"
      >
        <ChevronDown
          size={13}
          className={`shrink-0 text-ink-3 transition-transform duration-200 ease-out ${expanded ? '' : '-rotate-90'}`}
          aria-hidden
        />
        <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink" title={file.path}>
          {file.old_path ? `${file.old_path} → ` : ''}
          {file.path}
        </span>
        {status ? <span className="shrink-0 text-[11px] text-ink-3">{status}</span> : null}
        <FileCounts file={file} />
      </button>
      {expanded && file.binary ? (
        <div className="border-t border-border">
          <DiffView patch="" binary />
        </div>
      ) : null}
      {expanded && !file.binary ? <FileDiffBody sessionId={sessionId} base={base} file={file} /> : null}
    </section>
  )
}

function FileDiffBody({
  sessionId,
  base,
  file,
}: {
  sessionId: string
  base?: string
  file: RepoFileChange
}) {
  const diff = useQuery(sessionRepoFileDiffQuery(sessionId, file, base))
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
      <DiffView patch={diff.data.patch} binary={diff.data.binary} truncated={diff.data.truncated} />
    </div>
  )
}
