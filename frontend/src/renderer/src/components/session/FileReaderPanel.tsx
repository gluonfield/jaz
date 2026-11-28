import { useQuery } from '@tanstack/react-query'
import { FileText, LoaderCircle, Share2, X } from 'lucide-react'
import { lazy, memo, Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { useSaveApp } from '@/components/apps/useSaveApp'
import { HighlightedCodeLine, useSyntaxHighlightedLines } from '@/components/session/HighlightedCode'
import { DocumentEditor, DocumentHeader, isEditableDocument } from '@/components/document/DocumentEditor'
import { FileReaderLinkProvider, RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { ApiError } from '@/lib/api/client'
import { effectiveCloudRunId, grantSessionAsset, healthQuery, sessionFileBytes, sessionFileQuery, sessionFileRawUrl } from '@/lib/api/sessions'
import { useFileLocation } from '@/lib/hooks/useFileLocation'
import { useRefetchAfterWork } from '@/lib/hooks/useRefetchAfterWork'
import type { HealthResponse } from '@/lib/api/types'
import { markdownDocumentTitle } from '@/lib/markdownDocuments'
import { FileActions } from '@/components/session/FileActions'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { isMarkdownPath, isSpreadsheetPath, isWordPath, resolveFileLink, type FileLocation, type FileReference } from '@shared/fileReader'

const SpreadsheetFileView = lazy(() => import('@/components/session/SpreadsheetFileView'))
const PDFFileView = lazy(() => import('@/components/session/PDFFileView'))
const WordFileView = lazy(() => import('@/components/session/WordFileView'))

export const FileReaderPanel = memo(function FileReaderPanel({
  sessionId,
  cloudRunId,
  fileRef,
  visible,
  working = false,
  onOpenFile,
  onClose,
}: {
  sessionId: string
  cloudRunId?: string
  fileRef: FileReference | null
  visible: boolean
  working?: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const filePath = fileRef?.path ?? ''
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  const pdf = isPDFPath(filePath)
  const word = isWordPath(filePath)
  const loadBytes = useCallback((signal: AbortSignal) => sessionFileBytes(sessionId, filePath, signal, runId), [sessionId, filePath, runId])
  const resolvePdfUrl = useCallback(async (signal: AbortSignal) => {
    const trimmed = filePath.trim()
    if (!trimmed) return null
    if (!runId && (trimmed.startsWith('/') || trimmed.includes('\\') || /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed))) {
      return null
    }
    try {
      const asset = await grantSessionAsset(sessionId, trimmed, signal, runId)
      return asset.url || null
    } catch {
      return null
    }
  }, [sessionId, filePath, runId])
  const spreadsheet = isSpreadsheetPath(filePath)
  const scrollsInternally = Boolean(pdf || word || spreadsheet)
  const markdown = isMarkdownPath(filePath)
  const image = /\.(png|jpe?g|gif|webp|avif|bmp|svg|ico)(?:[?#].*)?$/i.test(filePath)
  // A document or a spreadsheet follows the agent while it writes, and once more when it stops.
  const follows = (markdown || spreadsheet) && working
  const file = useQuery({ ...sessionFileQuery(sessionId, filePath, runId), enabled: visible && Boolean(filePath) && !pdf && !word && !image, refetchInterval: follows ? 2000 : false })
  useRefetchAfterWork(follows, file.refetch)
  const health = useQuery({ ...healthQuery, enabled: visible && Boolean(filePath) && !runId })
  const [draft, setDraft] = useState(filePath)
  const [inputError, setInputError] = useState('')
  const saveApp = useSaveApp()
  const isHtmlOrSvg = /\.(html?|svg)$/i.test(filePath)
  const content = file.data?.binary ? undefined : file.data?.content
  const canPublish = (markdown || isHtmlOrSvg) && content !== undefined && !saveApp.isPending
  const publish = () => {
    if (content === undefined) return
    saveApp.mutate({
      title: markdown ? markdownDocumentTitle(content, filePath) : (filePath.split(/[\\/]/).at(-1) ?? 'App'),
      kind: markdown ? 'document' : 'artifact',
      entry: markdown ? 'index.md' : filePath.toLowerCase().endsWith('.svg') ? 'index.svg' : 'index.html',
      markdown: markdown ? content : undefined,
      html: markdown ? undefined : content,
      source_thread_id: sessionId,
    })
  }

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
    return <DocumentEditor key={`${sessionId}:${filePath}`} sessionId={sessionId} cloudRunId={runId} path={filePath} file={file.data} location={fileRef?.location} active={visible} onOpenFile={onOpenFile} onClose={onClose} />
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      {markdown ? (
        <DocumentHeader title={markdownDocumentTitle(file.data?.content ?? '', filePath)} onClose={onClose}>
          {canPublish ? (
            <Button size="sm" variant="ghost" onClick={publish} title="Save to Apps">
              <Share2 size={12} />
              Publish
            </Button>
          ) : null}
          <FileActions sessionId={sessionId} cloudRunId={runId} path={filePath} />
        </DocumentHeader>
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
        {canPublish ? (
          <Button size="sm" variant="ghost" type="button" onClick={publish} title="Save to Apps">
            <Share2 size={12} />
            Publish
          </Button>
        ) : null}
        {filePath ? <FileActions sessionId={sessionId} cloudRunId={runId} path={filePath} /> : null}
        <IconButton size="sm" aria-label="Close file" onClick={onClose}><X size={16} /></IconButton>
      </form>}
      {inputError ? (
        <p className="shrink-0 border-b border-border px-3 py-2 text-[12px] text-danger">
          {inputError}
        </p>
      ) : null}
      <div className={`min-h-0 flex-1 bg-bg ${scrollsInternally ? 'overflow-hidden' : 'scrollbar-quiet overflow-auto'}`}>
        {!filePath ? (
          <div className="flex h-full items-center justify-center px-8 text-center text-[13px] text-ink-3">
            No file selected.
          </div>
        ) : image ? (
          <ImageFileView key={`${sessionId}:${runId ?? ''}:${filePath}`} url={sessionFileRawUrl(sessionId, filePath, runId)} name={filePath.split(/[\\/]/).at(-1) ?? 'Image'} />
        ) : pdf ? (
          <Suspense fallback={<p className="p-3 text-[12px] text-ink-3">Loading PDF…</p>}>
            <PDFFileView key={`${sessionId}:${runId ?? ''}:${filePath}`} load={loadBytes} resolveUrl={resolvePdfUrl} location={fileRef?.location} visible={visible} />
          </Suspense>
        ) : word ? (
          <Suspense fallback={<p className="p-3 text-[12px] text-ink-3">Loading Word document…</p>}>
            <WordFileView key={`${sessionId}:${runId ?? ''}:${filePath}`} load={loadBytes} location={fileRef?.location} visible={visible} />
          </Suspense>
        ) : spreadsheet && !file.isPending ? (
          <Suspense fallback={<p className="p-3 text-[12px] text-ink-3">Loading spreadsheet…</p>}>
            <SpreadsheetFileView key={`${filePath}:${runId ?? ''}:${fileRef?.location?.sheet ?? ''}:${fileRef?.location?.range ?? ''}`} sessionId={sessionId} cloudRunId={runId} path={filePath} version={file.data?.version} visible={visible} initialSheet={fileRef?.location?.sheet} initialSelection={fileRef?.location?.range} />
          </Suspense>
        ) : file.isPending ? (
          <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" aria-hidden />
            Loading file…
          </div>
        ) : file.isError ? (
          !runId && health.isPending ? (
            <div className="flex items-center gap-2 px-3 py-4 text-[12px] text-ink-3">
              <LoaderCircle size={13} className="animate-spin" aria-hidden />
              Checking backend…
            </div>
          ) : !runId && unsupportedFileReader(file.error, health.data) ? (
            <p className="px-3 py-4 text-[12px] text-danger">
              This backend does not expose server-side file reading. Restart or update the Jaz server, then try again.
            </p>
          ) : (
            <p className="px-3 py-4 text-[12px] text-danger">
              {markdown ? 'Couldn’t open this document. Try opening it again.' : <>Couldn&apos;t open the file: {(file.error as Error).message}</>}
            </p>
          )
        ) : file.data.binary ? (
          <p className="px-3 py-4 text-[12px] text-ink-3">Binary file — no text preview.</p>
        ) : (
          <FilePreview
            sessionId={sessionId}
            cloudRunId={runId}
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
  cloudRunId,
  path,
  content,
  location,
  onOpenFile,
  active,
}: {
  sessionId: string
  cloudRunId?: string
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
      <FileReaderLinkProvider sessionId={sessionId} cloudRunId={cloudRunId} documentPath={path} onOpen={onOpenFile}>
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
