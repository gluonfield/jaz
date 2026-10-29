import { useQuery } from '@tanstack/react-query'
import { FileText, LoaderCircle, X } from 'lucide-react'
import { lazy, memo, Suspense, useEffect, useMemo, useState } from 'react'
import { HighlightedCodeLine, useSyntaxHighlightedLines } from '@/components/session/HighlightedCode'
import { IconButton } from '@/components/ui/IconButton'
import { FileReaderLinkProvider, RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { ApiError } from '@/lib/api/client'
import { healthQuery, sessionFileQuery, sessionFileRawUrl } from '@/lib/api/sessions'
import type { HealthResponse } from '@/lib/api/types'
import { markdownDocumentTitle } from '@/lib/markdownDocuments'
import { isMarkdownPath, isSpreadsheetPath, parseFileReference, type FileReference } from '@shared/fileReader'

const SpreadsheetFileView = lazy(() => import('@/components/session/SpreadsheetFileView'))

const FILE_LINE_SUFFIX = /^(.*?):(\d+)(?::\d+)?$/

export const FileReaderPanel = memo(function FileReaderPanel({
  sessionId,
  fileRef,
  visible,
  onOpenFile,
  onClose,
}: {
  sessionId: string
  fileRef: FileReference | null
  visible: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const filePath = fileRef?.path ?? ''
  const pdf = isPDFPath(filePath)
  const spreadsheet = isSpreadsheetPath(filePath)
  const markdown = isMarkdownPath(filePath)
  const file = useQuery({ ...sessionFileQuery(sessionId, filePath), enabled: visible && Boolean(filePath) && !spreadsheet })
  const health = useQuery({ ...healthQuery, enabled: visible && Boolean(filePath) })
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
    <div className="flex h-full min-h-0 flex-col">
      {markdown ? (
        <header className="flex h-11 shrink-0 items-center gap-2 border-b border-border px-3">
          <FileText size={15} className="shrink-0 text-ink-3" aria-hidden />
          <p className="min-w-0 flex-1 truncate text-[13px] font-medium">{markdownDocumentTitle(file.data?.content ?? '', filePath)}</p>
          <IconButton size="sm" aria-label="Close document" onClick={onClose}><X size={16} /></IconButton>
        </header>
      ) : <form
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
          placeholder="Path to a file"
          spellCheck={false}
          className="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink outline-none placeholder:text-ink-3"
        />
      </form>}
      {inputError ? (
        <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">
          {inputError}
        </p>
      ) : null}
      <div className={`min-h-0 flex-1 bg-bg ${pdf || spreadsheet ? 'overflow-hidden' : 'scrollbar-quiet overflow-auto'}`}>
        {!filePath ? (
          <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
            No file selected.
          </div>
        ) : spreadsheet ? (
          <Suspense fallback={<p className="p-3 text-[12px] text-ink-3">Loading spreadsheet…</p>}>
            <SpreadsheetFileView key={filePath} sessionId={sessionId} path={filePath} visible={visible} />
          </Suspense>
        ) : file.isPending ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading file…
          </div>
        ) : file.isError ? (
          health.isPending ? (
            <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
              <LoaderCircle size={13} className="animate-spin" aria-hidden />
              Checking backend…
            </div>
          ) : unsupportedFileReader(file.error, health.data) ? (
            <p className="px-3 py-4 text-[12px] text-danger">
              This backend does not expose server-side file reading. Restart or update the Jaz server, then try again.
            </p>
          ) : (
            <p className="px-3 py-4 text-[12px] text-danger">
              {markdown ? 'Couldn’t open this document. Try opening it again.' : <>Couldn&apos;t open the file: {(file.error as Error).message}</>}
            </p>
          )
        ) : pdf ? (
          <PDFFileView url={sessionFileRawUrl(sessionId, filePath)} />
        ) : file.data.binary ? (
          <p className="px-3 py-4 text-[12px] text-ink-3">Binary file — no text preview.</p>
        ) : (
          <FilePreview
            sessionId={sessionId}
            path={file.data.path}
            content={file.data.content ?? ''}
            highlightLine={fileRef?.line}
            onOpenFile={onOpenFile}
          />
        )}
      </div>
    </div>
  )
})

function FilePreview({
  sessionId,
  path,
  content,
  highlightLine,
  onOpenFile,
}: {
  sessionId: string
  path: string
  content: string
  highlightLine?: number
  onOpenFile: (file: FileReference) => void
}) {
  if (isMarkdownPath(path)) {
    return (
      <FileReaderLinkProvider sessionId={sessionId} documentPath={path} onOpen={onOpenFile}>
        <RenderedMarkdown text={content} className="file-prose" />
      </FileReaderLinkProvider>
    )
  }
  return <FileTextView path={path} content={content} highlightLine={highlightLine} />
}

function unsupportedFileReader(error: unknown, health?: HealthResponse): boolean {
  if (health?.capabilities?.session_file_read) return false
  return error instanceof ApiError && error.status === 404 && error.message.trim().toLowerCase() === 'not found'
}

function FileTextView({
  path,
  content,
  highlightLine,
}: {
  path: string
  content: string
  highlightLine?: number
}) {
  const lines = useMemo(() => content.split('\n'), [content])
  const highlighted = useSyntaxHighlightedLines(path, lines)
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
                  <HighlightedCodeLine text={line} tokens={highlighted?.[index]} />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function PDFFileView({ url }: { url: string }) {
  return (
    <iframe
      src={url}
      title="PDF file preview"
      referrerPolicy="no-referrer"
      className="h-full w-full border-0 bg-bg"
    />
  )
}

function isPDFPath(path: string): boolean {
  return path.toLowerCase().replace(/[?#].*$/, '').endsWith('.pdf')
}

function parseDraftReference(value: string): FileReference | null {
  let path = value.trim().replace(/[),.;]+$/, '')
  const lineMatch = FILE_LINE_SUFFIX.exec(path)
  const line = lineMatch ? Number(lineMatch[2]) : undefined
  if (lineMatch) path = lineMatch[1]
  return path ? { path, line } : null
}
