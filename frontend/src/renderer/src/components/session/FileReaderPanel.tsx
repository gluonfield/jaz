import { useQuery } from '@tanstack/react-query'
import { FileText, LoaderCircle } from 'lucide-react'
import { lazy, memo, Suspense, useEffect, useMemo, useState } from 'react'
import { HighlightedCodeLine, useSyntaxHighlightedLines } from '@/components/session/HighlightedCode'
import { DocumentEditor, DocumentHeader, isEditableDocument } from '@/components/document/DocumentEditor'
import { FileReaderLinkProvider, RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { ApiError } from '@/lib/api/client'
import { healthQuery, sessionFileQuery, sessionFileRawUrl } from '@/lib/api/sessions'
import { useFileLocation } from '@/lib/hooks/useFileLocation'
import { useRefetchAfterWork } from '@/lib/hooks/useRefetchAfterWork'
import type { HealthResponse } from '@/lib/api/types'
import { markdownDocumentTitle } from '@/lib/markdownDocuments'
import { FileActions } from '@/components/session/FileActions'
import { Button } from '@/components/ui/Button'
import { isMarkdownPath, isSpreadsheetPath, resolveFileLink, type FileLocation, type FileReference } from '@shared/fileReader'

const SpreadsheetFileView = lazy(() => import('@/components/session/SpreadsheetFileView'))

export const FileReaderPanel = memo(function FileReaderPanel({
  sessionId,
  fileRef,
  visible,
  working = false,
  onOpenFile,
  onClose,
}: {
  sessionId: string
  fileRef: FileReference | null
  visible: boolean
  working?: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const filePath = fileRef?.path ?? ''
  const pdf = isPDFPath(filePath)
  const spreadsheet = isSpreadsheetPath(filePath)
  const markdown = isMarkdownPath(filePath)
  const image = /\.(png|jpe?g|gif|webp|avif|bmp|svg|ico)(?:[?#].*)?$/i.test(filePath)
  // A document follows the agent while it writes, and once more when it stops.
  const file = useQuery({ ...sessionFileQuery(sessionId, filePath), enabled: visible && Boolean(filePath) && !spreadsheet && !image, refetchInterval: markdown && working ? 2000 : false })
  useRefetchAfterWork(markdown && working, file.refetch)
  const health = useQuery({ ...healthQuery, enabled: visible && Boolean(filePath) })
  const [draft, setDraft] = useState(filePath)
  const [inputError, setInputError] = useState('')

  useEffect(() => {
    setDraft(filePath)
    setInputError('')
  }, [filePath])

  const submit = () => {
    const next = resolveFileLink(draft) ?? (draft.trim() ? { path: draft.trim() } : null)
    if (!next) {
      setInputError('Enter a file path.')
      return
    }
    setInputError('')
    onOpenFile(next)
  }

  if (markdown && file.data && isEditableDocument(file.data)) {
    return <DocumentEditor key={`${sessionId}:${filePath}`} sessionId={sessionId} path={filePath} file={file.data} location={fileRef?.location} active={visible} onOpenFile={onOpenFile} onClose={onClose} />
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      {markdown ? (
        <DocumentHeader title={markdownDocumentTitle(file.data?.content ?? '', filePath)} onClose={onClose}>
          <FileActions sessionId={sessionId} path={filePath} />
        </DocumentHeader>
      ) : <form
        onSubmit={(event) => {
          event.preventDefault()
          submit()
        }}
        className="@container/file flex h-11 shrink-0 items-center gap-2 border-b border-border px-3"
      >
        <FileText size={15} className="shrink-0 text-ink-3" aria-hidden />
        <input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="Path to a file"
          spellCheck={false}
          className="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink outline-none placeholder:text-ink-3"
        />
        {filePath ? <FileActions sessionId={sessionId} path={filePath} /> : null}
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
        ) : image ? (
          <ImageFileView key={`${sessionId}:${filePath}`} url={sessionFileRawUrl(sessionId, filePath)} name={filePath.split(/[\\/]/).at(-1) ?? 'Image'} />
        ) : spreadsheet ? (
          <Suspense fallback={<p className="p-3 text-[12px] text-ink-3">Loading spreadsheet…</p>}>
            <SpreadsheetFileView key={`${filePath}:${fileRef?.location?.sheet ?? ''}:${fileRef?.location?.range ?? ''}`} sessionId={sessionId} path={filePath} visible={visible} initialSheet={fileRef?.location?.sheet} initialSelection={fileRef?.location?.range} />
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
          <PDFFileView url={sessionFileRawUrl(sessionId, filePath)} location={fileRef?.location} />
        ) : file.data.binary ? (
          <p className="px-3 py-4 text-[12px] text-ink-3">Binary file — no text preview.</p>
        ) : (
          <FilePreview
            sessionId={sessionId}
            path={file.data.path}
            content={file.data.content ?? ''}
            location={fileRef?.location}
            onOpenFile={onOpenFile}
            active={visible}
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
  location,
  onOpenFile,
  active,
}: {
  sessionId: string
  path: string
  content: string
  location?: FileLocation
  onOpenFile: (file: FileReference) => void
  active: boolean
}) {
  const [root, setRoot] = useState<HTMLDivElement | null>(null)
  useFileLocation(root, location, content, active)
  if (isMarkdownPath(path)) {
    return (
      <FileReaderLinkProvider sessionId={sessionId} documentPath={path} onOpen={onOpenFile}>
        <div ref={setRoot}><RenderedMarkdown text={content} className="file-prose" artifactPreview={!active} /></div>
      </FileReaderLinkProvider>
    )
  }
  return <div ref={setRoot}><FileTextView path={path} content={content} /></div>
}

function unsupportedFileReader(error: unknown, health?: HealthResponse): boolean {
  if (health?.capabilities?.session_file_read) return false
  return error instanceof ApiError && error.status === 404 && error.message.trim().toLowerCase() === 'not found'
}

function FileTextView({
  path,
  content,
}: {
  path: string
  content: string
}) {
  const lines = useMemo(() => content.split('\n'), [content])
  const highlighted = useSyntaxHighlightedLines(path, lines)
  return (
    <div className="overflow-x-auto bg-bg/45 font-mono text-[12px] leading-[1.55] select-text">
      <table className="w-full min-w-max border-separate border-spacing-0">
        <tbody>
          {lines.map((line, index) => {
            const lineNo = index + 1
            return (
              <tr key={index} data-file-line={lineNo}>
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

function PDFFileView({ url, location }: { url: string; location?: FileLocation }) {
  const fragment = new URLSearchParams({ toolbar: '0', navpanes: '0' })
  if (location?.page) fragment.set('page', String(location.page))
  return (
    <iframe
      src={url + (fragment.size ? '#' + fragment : '')}
      title="PDF file preview"
      referrerPolicy="no-referrer"
      className="h-full w-full border-0 bg-bg"
    />
  )
}

function ImageFileView({ url, name }: { url: string; name: string }) {
  const [failedSource, setFailedSource] = useState('')
  return failedSource === url ? <div className="flex items-center gap-2 px-3 py-4 text-[12px]">
    <p className="text-danger">Couldn’t open this image.</p>
    <Button size="sm" onClick={() => setFailedSource('')}>Retry</Button>
  </div> : (
    <div className="flex h-full items-center justify-center p-4">
      <img src={url} alt={name} referrerPolicy="no-referrer" onError={() => setFailedSource(url)}
        className="max-h-full max-w-full object-contain outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10" />
    </div>
  )
}

function isPDFPath(path: string): boolean {
  return path.toLowerCase().replace(/[?#].*$/, '').endsWith('.pdf')
}
