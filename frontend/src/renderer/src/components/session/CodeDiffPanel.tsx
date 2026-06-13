import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FolderGit2, LoaderCircle, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { sessionRepoChangesQuery, sessionRepoFileDiffQuery, sessionRepoQuery } from '@/lib/api/sessions'
import { fileKey, type RepoFileChange, type Session } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

export const CODE_DIFF_PANEL_WIDTH = 640

export function CodeDiffPanel({
  session,
  visible,
  onClose,
}: {
  session: Session
  visible: boolean
  onClose: () => void
}) {
  const changes = useQuery({ ...sessionRepoChangesQuery(session.id), enabled: visible })
  const repo = useQuery({ ...sessionRepoQuery(session.id), enabled: visible })
  const data = changes.data
  const firstKey = data?.files[0] ? fileKey(data.files[0]) : ''
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  useEffect(() => setExpanded({}), [session.id])
  useEffect(() => {
    if (!firstKey) return
    setExpanded((current) => (Object.keys(current).length ? current : { [firstKey]: true }))
  }, [firstKey])

  const summary = useMemo(() => {
    if (!data) return 'Code diff'
    const files = `${data.files.length} ${data.files.length === 1 ? 'file' : 'files'}`
    return `${files} · +${data.total_added} −${data.total_deleted}`
  }, [data])
  const base = shortRef(repo.data?.main_branch || repo.data?.default_branch || data?.base || 'main')

  return (
    <aside
      style={{ width: CODE_DIFF_PANEL_WIDTH }}
      className="flex h-full shrink-0 flex-col bg-bg p-2"
    >
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[14px] bg-surface shadow-[0_18px_46px_rgba(0,0,0,0.18)] ring-1 ring-border">
        <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border px-3">
          <FolderGit2 size={15} className="shrink-0 text-ink-3" aria-hidden />
          <div className="flex min-w-0 flex-1 items-baseline gap-1.5">
            <span className="min-w-0 truncate font-mono text-[13px] text-ink-2">{base}</span>
            <span className="text-[13px] text-ink-3" aria-hidden>
              →
            </span>
            <span className="shrink-0 text-[13px] text-ink-2">working tree</span>
            <span className="ml-1 hidden shrink-0 font-mono text-[11px] text-ink-3 tabular-nums sm:inline">
              {summary}
            </span>
          </div>
          <button
            type="button"
            aria-label="Hide side panel"
            onClick={onClose}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
          >
            <X size={15} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {changes.isPending ? (
            <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
              <LoaderCircle size={13} className="animate-spin" aria-hidden />
              Loading changes…
            </div>
          ) : null}
          {data && data.files.length === 0 ? (
            <div className="px-3 py-4 text-[12px] text-ink-3">No code changes to show.</div>
          ) : null}
          {data?.files.length
            ? data.files.map((file) => {
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
              })
            : null}
        </div>
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

function shortRef(value: string): string {
  return value.length > 18 && /^[a-f0-9]+$/i.test(value) ? value.slice(0, 7) : value
}
