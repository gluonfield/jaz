import { useEffect, useRef, useState } from 'react'
import { AnnotationMode, getDocument, PDFWorker, type PDFDocumentLoadingTask, type PDFDocumentProxy } from 'pdfjs-dist'
import { EventBus, PDFViewer } from 'pdfjs-dist/web/pdf_viewer.mjs'
import PdfWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?worker'
import type { FileLocation } from '@shared/fileReader'
import { pdfLocation, pdfTextRange } from '@/lib/pdfLocation'
import { Button } from '@/components/ui/Button'
import './pdf.css'

type DocumentView = { viewer: PDFViewer; document: PDFDocumentProxy; events: EventBus }

export default function PDFFileView({ url, location, visible = true }: { url: string | (() => string); location?: FileLocation; visible?: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [view, setView] = useState<DocumentView | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [target, setTarget] = useState<{ page?: number; quote?: string; location?: FileLocation } | null>(null)
  const applied = useRef<typeof target>(null)

  useEffect(() => {
    if (!container) return
    const abort = new AbortController()
    const port = new PdfWorker()
    port.addEventListener('error', () => { if (!abort.signal.aborted) setError('Couldn’t load the PDF renderer.') })
    const worker = PDFWorker.create({ port })
    const events = new EventBus()
    const options = { container, eventBus: events, annotationMode: AnnotationMode.DISABLE, enableSelectionRendering: false, enableAutoLinking: false, abortSignal: abort.signal }
    const viewer = new PDFViewer(options)
    let task: PDFDocumentLoadingTask | undefined
    let width = 0
    const fit = () => {
      const next = container.clientWidth
      if (viewer.pdfDocument && next && next !== width) {
        width = next
        viewer.currentScaleValue = 'page-width'
      }
    }
    const observer = new ResizeObserver(fit)
    observer.observe(container)
    events.on('pagesinit', fit)
    setError('')
    setView(null)
    setTarget(null)
    applied.current = null
    void (async () => {
      const response = await fetch(typeof url === 'function' ? url() : url, { signal: abort.signal })
      if (!response.ok) throw new Error(`Couldn’t open PDF (${response.status}).`)
      const data = new Uint8Array(await response.arrayBuffer())
      if (abort.signal.aborted) return
      task = getDocument({ data, worker })
      const document = await task.promise
      if (abort.signal.aborted) return
      events.on('pagesinit', () => setView({ viewer, document, events }))
      viewer.setDocument(document)
    })().catch((error: Error) => { if (!abort.signal.aborted) setError(error.message) })
    return () => {
      abort.abort()
      observer.disconnect()
      viewer.setDocument(null)
      const closeWorker = () => {
        worker.destroy()
        port.terminate()
      }
      if (task) void task.destroy().finally(closeWorker)
      else closeWorker()
    }
  }, [container, url, attempt])

  useEffect(() => {
    if (!view) return
    setTarget(null)
    const abort = new AbortController()
    void pdfLocation(view.document, location, abort.signal).then((destination) => {
      if (!abort.signal.aborted) setTarget({ ...destination, location })
    }).catch((error: Error) => { if (!abort.signal.aborted) setError(error.message) })
    return () => abort.abort()
  }, [view, location])

  useEffect(() => {
    if (!view || !target || !visible || applied.current === target) return
    if (target.page) view.viewer.scrollPageIntoView({ pageNumber: target.page })
    if (!target.quote) applied.current = target
  }, [view, target, visible])

  useEffect(() => {
    if (!view || !target?.page || !target.quote) return
    const { page, quote } = target
    let range: Range | null = null
    const clear = () => {
      const highlight = CSS.highlights.get('pdf-location')
      if (range) highlight?.delete(range)
      if (highlight && !highlight.size) CSS.highlights.delete('pdf-location')
      range = null
    }
    const mark = () => {
      const element = view.viewer.getPageView(page - 1)?.div as HTMLDivElement | undefined
      const layer = element?.querySelector<HTMLElement>('.textLayer')
      if (!layer) return
      clear()
      range = pdfTextRange(layer, quote)
      if (!range) return
      const highlight = CSS.highlights.get('pdf-location') ?? new Highlight()
      highlight.add(range)
      CSS.highlights.set('pdf-location', highlight)
      if (visible && applied.current !== target) {
        range.startContainer.parentElement?.scrollIntoView({ block: 'center' })
        applied.current = target
      }
    }
    view.events.on('textlayerrendered', mark)
    mark()
    return () => {
      view.events.off('textlayerrendered', mark)
      clear()
    }
  }, [view, target, visible])

  return <div className="pdf-file-view relative h-full min-h-0 bg-bg text-ink">
    <div ref={setContainer} className="absolute inset-0 overflow-y-auto overflow-x-hidden" role="region" aria-label="PDF document">
      <div className="pdfViewer" />
    </div>
    {error ? <div className="relative flex items-center gap-2 bg-bg px-3 py-4 text-[12px]">
      <p role="alert" className="text-danger">{error}</p>
      <Button size="sm" onClick={() => setAttempt((value) => value + 1)}>Retry</Button>
    </div> : !view ? <p role="status" className="relative px-3 py-4 text-[12px] text-ink-3">Loading PDF…</p> : null}
  </div>
}
