import { defaultOptions, renderAsync, type Options } from 'docx-preview'
import { useEffect, useRef, useState } from 'react'
import { fileTextRange } from '@/lib/fileTextLocation'
import type { FileLocation } from '@shared/fileReader'

const frameSource = `<!doctype html><html lang="en"><head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
  <style>
    html { color-scheme: light; background: #e7e9ec; scrollbar-width: thin; }
    body { margin: 0; }
    body .docx-wrapper { background: transparent; padding: 12px; align-items: safe center; }
    body .docx-wrapper > section.docx { margin-bottom: 12px; box-shadow: 0 1px 5px #0002; }
    ::highlight(file-location) { background: #ffd56a; color: #171717; }
  </style>
</head><body></body></html>`

function wordNode(document: Document, value: Parameters<Options['h']>[0]): Node {
  if (typeof value === 'string') return document.createTextNode(value)
  if (value instanceof Node || value instanceof document.defaultView!.Node) return value
  const { children, ...element } = value
  if (element.tagName === '#comment') return document.createComment('')
  // Adopt each node before the renderer assigns image URLs or assembles its children.
  const node = document.adoptNode(defaultOptions.h({ ...element, href: undefined }))
  for (const child of children ?? []) node.appendChild(wordNode(document, child))
  return node
}

export default function WordFileView({ load, location, visible }: {
  load: (signal: AbortSignal) => Promise<ArrayBuffer>
  location?: FileLocation
  visible: boolean
}) {
  const [document, setDocument] = useState<Document | null>(null)
  const [content, setContent] = useState<HTMLElement | null>(null)
  const [failed, setFailed] = useState(false)
  const applied = useRef<{ content: HTMLElement; location?: FileLocation } | null>(null)

  useEffect(() => {
    if (!document) return
    const abort = new AbortController()
    let observer: ResizeObserver | undefined
    void (async () => {
      const data = await load(abort.signal)
      if (abort.signal.aborted) return
      const rendered = document.createElement('div')
      await renderAsync(data, rendered, undefined, {
        renderAltChunks: false,
        useBase64URL: true,
        ignoreLastRenderedPageBreak: false,
        h: (value) => wordNode(document, value),
      })
      if (abort.signal.aborted) return
      const wrapper = rendered.querySelector<HTMLElement>('.docx-wrapper')
      if (!wrapper) throw new Error('Missing Word document content')
      document.body.replaceChildren(...rendered.childNodes)
      let width = 0
      const fit = () => {
        const available = document.documentElement.clientWidth - 24
        if (available <= 0) return
        width ||= Array.from(wrapper.children).reduce((max, page) => Math.max(max, page.getBoundingClientRect().width), 1)
        const zoom = String(Math.max(0.75, Math.min(1, available / width)))
        if (wrapper.style.zoom !== zoom) wrapper.style.zoom = zoom
      }
      fit()
      observer = new ResizeObserver(fit)
      observer.observe(document.documentElement)
      setContent(wrapper)
    })().catch(() => { if (!abort.signal.aborted) setFailed(true) })
    return () => {
      abort.abort()
      observer?.disconnect()
    }
  }, [document, load])

  useEffect(() => {
    if (!content || !visible || applied.current?.content === content && applied.current.location === location) return
    const view = content.ownerDocument.defaultView!
    const highlights = view.CSS.highlights
    const range = location?.text ? fileTextRange(content, location.text) : null
    highlights?.delete('file-location')
    if (range && highlights) highlights.set('file-location', new view.Highlight(range))
    const rect = range?.getClientRects()[0]
    view.scrollTo(
      rect ? view.scrollX + rect.left - (view.innerWidth - rect.width) / 2 : 0,
      rect ? view.scrollY + rect.top - (view.innerHeight - rect.height) / 2 : 0,
    )
    applied.current = { content, location }
  }, [content, location, visible])

  return <div className="relative h-full">
    <iframe
      className={`absolute inset-0 size-full border-0 ${content ? '' : 'invisible'}`}
      title="Word document"
      sandbox="allow-same-origin"
      srcDoc={frameSource}
      onLoad={(event) => setDocument(event.currentTarget.contentDocument)}
    />
    {failed ? <p role="alert" className="px-3 py-4 text-[12px] text-danger">Couldn’t open this Word document.</p>
      : !content ? <p role="status" className="px-3 py-4 text-[12px] text-ink-3">Loading Word document…</p> : null}
  </div>
}
