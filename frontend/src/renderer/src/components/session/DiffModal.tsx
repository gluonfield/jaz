import { useQuery } from '@tanstack/react-query'
import { ChevronDown, FileDiff, LoaderCircle } from 'lucide-react'
import { Modal } from '@/components/ui/Modal'
import { sessionRepoFileDiffQuery } from '@/lib/api/sessions'
import { fileKey, type RepoChanges, type RepoFileChange } from '@/lib/api/types'
import { DiffView, FileCounts } from './DiffView'

// Codex-style review surface: every changed file as a collapsible section.
// Fully controlled — the opener owns which sections are expanded — and patch
// text is fetched per file only while its section is open, so the modal
// costs nothing beyond the already-loaded summary.
export function DiffModal({
  sessionId,
  changes,
  expanded,
  onToggle,
  open,
  onClose,
}: {
  sessionId: string
  changes: RepoChanges
  expanded: Record<string, boolean>
  onToggle: (key: string) => void
  open: boolean
  onClose: () => void
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Review changes"
      description={`${changes.files.length} ${changes.files.length === 1 ? 'file' : 'files'} · +${changes.total_added} −${changes.total_deleted}`}
      icon={<FileDiff size={16} />}
      size="xl"
    >
      <div className="flex flex-col gap-2">
        {changes.files.map((file) => {
          const key = fileKey(file)
          return (
            <FileSection
              key={key}
              sessionId={sessionId}
              base={changes.base}
              file={file}
              expanded={Boolean(expanded[key])}
              onToggle={() => onToggle(key)}
            />
          )
        })}
      </div>
    </Modal>
  )
}

const STATUS_LABEL: Record<RepoFileChange['status'], string> = {
  added: 'added',
  untracked: 'new',
  deleted: 'deleted',
  renamed: 'renamed',
  modified: '',
}

function FileSection({
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
    <section className="overflow-hidden rounded-card border border-border">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={onToggle}
        className="flex w-full cursor-pointer items-center gap-2 bg-surface px-3 py-2 text-left transition-colors hover:bg-surface-2"
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
      {/* The summary already knows binaries; don't fetch a patch that can't exist. */}
      {expanded && file.binary ? (
        <div className="border-t border-border">
          <DiffView patch="" binary />
        </div>
      ) : null}
      {expanded && !file.binary ? <FileDiffBody sessionId={sessionId} base={base} file={file} /> : null}
    </section>
  )
}

// Mounted only while its section is expanded, so the query never runs for
// files nobody opened.
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
