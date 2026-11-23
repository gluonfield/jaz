import { useEffect, useRef, useState } from 'react'
import { AnnotationMode, getDocument, PDFWorker, type PDFDocumentLoadingTask, type PDFDocumentProxy } from 'pdfjs-dist'
import { EventBus, PDFFindController, PDFLinkService, PDFViewer } from 'pdfjs-dist/web/pdf_viewer.mjs'
import PdfWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?worker'
import type { FileLocation } from '@shared/fileReader'
import { fileTextRange } from '@/lib/fileTextLocation'
import { pdfLocation } from '@/lib/pdfLocation'
import { Button } from '@/components/ui/Button'
import { FindBar } from './FindBar'
import { usePDFFind } from './usePDFFind'
import './pdf.css'

// Served by the pdfjsData build plugin; the worker fetches them, so the addresses are absolute.
const folder = new URL(`${import.meta.env.BASE_URL}pdfjs/`, document.baseURI).href
const sources = { cMapUrl: `${folder}cmaps/`, iccUrl: `${folder}iccs/`, standardFontDataUrl: `${folder}standard_fonts/`, wasmUrl: `${folder}wasm/` }

type DocumentView = { viewer: PDFViewer; document: PDFDocumentProxy; events: EventBus }

export default function PDFFileView({ load, location, visible = true }: { load: (signal: AbortSignal) => Promise<ArrayBuffer>; location?: FileLocation; visible?: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [view, setView] = useState<DocumentView | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [target, setTarget] = useState<{ page?: number; quote?: string; location?: FileLocation } | null>(null)
  const applied = useRef<typeof target>(null)
  const find = usePDFFind(view?.events, container)

  useEffect(() => {
    if (!container) return
    const abort = new AbortController()
    const port = new PdfWorker()
    port.addEventListener('error', () => { if (!abort.signal.aborted) setError('Couldn’t load the PDF renderer.') })
    const worker = PDFWorker.create({ port })
    const events = new EventBus()
    const linkService = new PDFLinkService({ eventBus: events })
    const findController = new PDFFindController({ linkService, eventBus: events })
    const options = { container, eventBus: events, linkService, findController, annotationMode: AnnotationMode.ENABLE, enableAutoLinking: false, abortSignal: abort.signal }
    const viewer = new PDFViewer(options)
    linkService.setViewer(viewer)
    let task: PDFDocumentLoadingTask | undefined
    let width = 0
    // Pages first fit the panel's width and then keep a zoom the person chose.
    const fit = () => {
      const next = container.clientWidth
      if (viewer.pdfDocument && next && next !== width) {
        width = next
        viewer.currentScaleValue = viewer.currentScaleValue ?? 'page-width'
      }
    }
    const observer = new ResizeObserver(fit)
    observer.observe(container)
    events.on('pagesinit', fit)
    // A pinch arrives as a wheel event with Control held, as does a mouse
    // wheel turned with Control, whose far larger steps are capped. pdf.js
    // rounds each scale to a hundredth, so the part of a small step it drops
    // is carried to the next.
    let carried = 1
    container.addEventListener('wheel', (event) => {
      if (!event.ctrlKey) return
      event.preventDefault()
      const factor = Math.exp(-Math.max(-25, Math.min(25, event.deltaY)) / 100)
      if ((carried > 1) !== (factor > 1)) carried = 1
      const scale = viewer.currentScale
      viewer.updateScale({ scaleFactor: carried * factor, origin: [event.clientX, event.clientY], drawingDelay: 400 })
      carried *= factor * scale / viewer.currentScale
    }, { passive: false, signal: abort.signal })
    const keys: Record<string, () => void> = {
      '=': () => viewer.increaseScale(),
      '+': () => viewer.increaseScale(),
      '-': () => viewer.decreaseScale(),
      '0': () => { viewer.currentScaleValue = 'page-width' },
    }
    container.addEventListener('keydown', (event) => {
      const zoom = (event.metaKey || event.ctrlKey) && !event.altKey && keys[event.key]
      if (!zoom) return
      event.preventDefault()
      zoom()
    }, { signal: abort.signal })
    setError('')
    setView(null)
    setTarget(null)
    applied.current = null
    void (async () => {
      const data = new Uint8Array(await load(abort.signal))
      if (abort.signal.aborted) return
      task = getDocument({ data, worker, ...sources })
      const document = await task.promise
      if (abort.signal.aborted) return
      events.on('pagesinit', () => setView({ viewer, document, events }))
      viewer.setDocument(document)
      linkService.setDocument(document)
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
  }, [container, load, attempt])

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
      range = fileTextRange(layer, quote)
      if (!range) return
      const highlight = CSS.highlights.get('pdf-location') ?? new Highlight()
      highlight.add(range)
      CSS.highlights.set('pdf-location', highlight)
      if (visible && applied.current !== target) {
        range.startContainer.parentElement?.scrollIntoView({ block: 'center' })
        applied.current = target
      }
    }
    // A search rewrites the page's text layer, which empties the range.
    let live = true
    const restore = () => queueMicrotask(() => { if (live) mark() })
    view.events.on('textlayerrendered', mark)
    view.events.on('updatetextlayermatches', restore)
    mark()
    return () => {
      live = false
      view.events.off('textlayerrendered', mark)
      view.events.off('updatetextlayermatches', restore)
      clear()
    }
  }, [view, target, visible])

  return <div className="pdf-file-view relative h-full min-h-0 bg-bg text-ink" onKeyDownCapture={find.handleKeyDownCapture}>
    <div ref={setContainer} tabIndex={0} className="absolute inset-0 overflow-auto outline-none" role="region" aria-label="PDF document">
      <div className="pdfViewer" />
    </div>
    <FindBar find={find} />
    {error ? <div className="relative flex items-center gap-2 bg-bg px-3 py-4 text-[12px]">
      <p role="alert" className="text-danger">{error}</p>
      <Button size="sm" onClick={() => setAttempt((value) => value + 1)}>Retry</Button>
    </div> : !view ? <p role="status" className="relative px-3 py-4 text-[12px] text-ink-3">Loading PDF…</p> : null}
  </div>
}
