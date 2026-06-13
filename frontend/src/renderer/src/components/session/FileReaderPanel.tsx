import { useQuery } from '@tanstack/react-query'
import { FileText, LoaderCircle, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { sessionFileQuery } from '@/lib/api/sessions'
import type { Session } from '@/lib/api/types'
import { parseFileReference, type FileReference } from '../../../../shared/fileReader'

export const FILE_READER_PANEL_WIDTH = 640
const FILE_LINE_SUFFIX = /^(.*?):(\d+)(?::\d+)?$/

export function FileReaderPanel({
  session,
  fileRef,
  visible,
  onOpenFile,
  onClose,
}: {
  session: Session
  fileRef: FileReference | null
  visible: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const filePath = fileRef?.path ?? ''
  const file = useQuery({ ...sessionFileQuery(session.id, filePath), enabled: visible && Boolean(filePath) })
  const [draft, setDraft] = useState(filePath)
  const [inputError, setInputError] = useState('')

  useEffect(() => {
    setDraft(filePath)
    setInputError('')
  }, [filePath])

  const submit = () => {
    const next = parseFileReference(draft) ?? parseDraftReference(draft)
    if (!next) {
      setInputError('Enter a file path.')
      return
    }
    setInputError('')
    onOpenFile(next)
  }

  return (
    <aside
      style={{ width: FILE_READER_PANEL_WIDTH }}
      className="flex h-full shrink-0 flex-col border-l border-border bg-bg p-2"
    >
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[14px] bg-surface shadow-[0_18px_46px_rgba(0,0,0,0.18)] ring-1 ring-border">
        <form
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
          className="flex h-11 shrink-0 items-center gap-2 border-b border-border px-3"
        >
          <FileText size={15} className="shrink-0 text-ink-3" aria-hidden />
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="/Users/wins/project/src/file.ts"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink outline-none placeholder:text-ink-3"
          />
          <button
            type="button"
            aria-label="Hide side panel"
            onClick={onClose}
            className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
          >
            <X size={15} />
          </button>
        </form>
        {inputError ? (
          <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">
            {inputError}
          </p>
        ) : null}
        <div className="min-h-0 flex-1 overflow-auto bg-bg">
          {!filePath ? (
            <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
              No file selected.
            </div>
          ) : file.isPending ? (
            <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
              <LoaderCircle size={13} className="animate-spin" aria-hidden />
              Loading file…
            </div>
          ) : file.isError ? (
            <p className="px-3 py-4 text-[12px] text-danger">
              Couldn&apos;t open the file: {(file.error as Error).message}
            </p>
          ) : file.data.binary ? (
            <p className="px-3 py-4 text-[12px] text-ink-3">Binary file — no text preview.</p>
          ) : (
            <>
              <div className="flex h-9 items-center gap-2 border-b border-border bg-surface px-3">
                <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink-2">
                  {file.data.relative_path || file.data.path}
                </span>
                <span className="shrink-0 font-mono text-[11px] text-ink-3 tabular-nums">
                  {formatBytes(file.data.size)}
                  {file.data.truncated ? ' · truncated' : ''}
                </span>
              </div>
              <FileTextView content={file.data.content ?? ''} highlightLine={fileRef?.line} />
            </>
          )}
        </div>
      </div>
    </aside>
  )
}

function FileTextView({ content, highlightLine }: { content: string; highlightLine?: number }) {
  const lines = content.split('\n')
  return (
    <div className="overflow-x-auto bg-bg/45 font-mono text-[12px] leading-[1.55] select-text">
      <table className="w-full min-w-max border-separate border-spacing-0">
        <tbody>
          {lines.map((line, index) => {
            const lineNo = index + 1
            const active = highlightLine === lineNo
            return (
              <tr key={index} className={active ? 'bg-primary-soft/70' : undefined}>
                <td className="w-12 min-w-12 pr-2 text-right align-top text-[11px] text-ink-3 tabular-nums select-none">
                  {lineNo}
                </td>
                <td className="whitespace-pre pr-5 align-top text-ink-2 select-text">
                  {line || ' '}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function parseDraftReference(value: string): FileReference | null {
  let path = value.trim().replace(/[),.;]+$/, '')
  const lineMatch = FILE_LINE_SUFFIX.exec(path)
  const line = lineMatch ? Number(lineMatch[2]) : undefined
  if (lineMatch) path = lineMatch[1]
  return path ? { path, line } : null
}
