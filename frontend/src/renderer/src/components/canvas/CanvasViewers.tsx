import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { buildArtifactDocument, buildComponentDocument } from '@/lib/artifacts'
import type { CanvasFile } from '@/lib/api/canvas'
import { readSessionFile } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { extension, fileName, viewerFor } from './board'
import { useCanvas } from './canvasContext'
import { FitContent } from '@/components/canvas/FitContent'
import { rawUrl, ViewerFrame } from './ViewerFrame'

const CadViewer = lazy(() => import('./CadViewer'))
const DxfViewer = lazy(() => import('./DxfViewer'))

export function FileViewer({ id, file, data }: { id: string; file: CanvasFile; data: unknown }) {
  const { sessionId, view, focused: focusedId } = useCanvas()
  const focused = focusedId === id
  if (!file.version) {
    return <ViewerFrame id={id}><div className="canvas-viewer-empty">File missing</div></ViewerFrame>
  }
  switch (viewerFor(file.path)) {
    case 'component':
      return <ComponentFrame id={id} code={view.components[id]?.code} error={view.components[id]?.error} data={data} />
    case 'image':
      return (
        <ViewerFrame id={id}>
          <img className="canvas-image" src={rawUrl(sessionId, file)} alt={fileName(file.path)} draggable={false} />
        </ViewerFrame>
      )
    case 'pdf':
      return (
        <ViewerFrame id={id} focusable paper>
          <iframe className="canvas-frame" title={fileName(file.path)} src={`${rawUrl(sessionId, file)}#${focused ? 'view=FitH' : 'toolbar=0&view=FitH'}`} />
        </ViewerFrame>
      )
    case 'cad':
      return (
        <Suspense fallback={<ViewerFrame id={id} focusable><div className="canvas-viewer-empty">Opening…</div></ViewerFrame>}>
          <CadViewer id={id} src={rawUrl(sessionId, file)} kind={extension(file.path)} focused={focused} />
        </Suspense>
      )
    case 'dxf':
      return (
        <Suspense fallback={<ViewerFrame id={id} focusable paper><div className="canvas-viewer-empty">Opening…</div></ViewerFrame>}>
          <DxfViewer id={id} src={rawUrl(sessionId, file)} />
        </Suspense>
      )
    case 'markdown':
    case 'csv':
    case 'html':
      return <TextFileViewer id={id} file={file} data={data} />
    case 'file':
      return <ViewerFrame id={id}><div className="canvas-viewer-empty">{extension(file.path).toUpperCase() || 'File'}</div></ViewerFrame>
  }
}

function TextFileViewer({ id, file, data }: { id: string; file: CanvasFile; data: unknown }) {
  const { sessionId, theme } = useCanvas()
  const read = useQuery({
    queryKey: [...keys.sessionFile(sessionId, file.path), file.version],
    queryFn: () => readSessionFile(sessionId, file.path),
    staleTime: Infinity,
  })
  const viewer = viewerFor(file.path)
  const text = read.data?.content ?? ''
  if (read.isError) return <ViewerFrame id={id}><div className="canvas-viewer-empty">{read.error.message}</div></ViewerFrame>
  if (viewer === 'html') {
    return (
      <ViewerFrame id={id}>
        <SandboxFrame id={id} title={fileName(file.path)} source={text + theme} data={data} build={(saved) => buildArtifactDocument({ title: fileName(file.path), code: text, loadingMessages: [] }, theme, { canvas: { data: saved } })} />
      </ViewerFrame>
    )
  }
  return (
    <ViewerFrame id={id}>
      <div className="canvas-text-file nodrag nopan"><FitContent>{viewer === 'csv' ? <CsvTable text={text} /> : <MessageMarkdown text={text} />}</FitContent></div>
    </ViewerFrame>
  )
}

function CsvTable({ text }: { text: string }) {
  const rows = text.trim().split(/\r?\n/).map(splitCsvLine)
  const [head, ...body] = rows
  return (
    <table className="canvas-csv">
      <thead>
        <tr>{head?.map((cell, i) => <th key={i}>{cell}</th>)}</tr>
      </thead>
      <tbody>
        {body.map((row, r) => (
          <tr key={r}>{row.map((cell, i) => <td key={i}>{cell}</td>)}</tr>
        ))}
      </tbody>
    </table>
  )
}

function splitCsvLine(line: string): string[] {
  const cells: string[] = []
  let cell = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const char = line[i]
    if (char === '"' && quoted && line[i + 1] === '"') {
      cell += '"'
      i++
    } else if (char === '"') {
      quoted = !quoted
    } else if (char === ',' && !quoted) {
      cells.push(cell)
      cell = ''
    } else {
      cell += char
    }
  }
  return [...cells, cell]
}

function ComponentFrame({ id, code, error, data }: { id: string; code?: string; error?: string; data: unknown }) {
  const { theme } = useCanvas()
  if (!code) {
    return <div className="canvas-card canvas-component-error">{error || 'Compiling…'}</div>
  }
  return (
    <div className="canvas-card">
      <SandboxFrame key={id} id={id} title={id} source={code + theme} data={data} build={(saved) => buildComponentDocument(code, theme, saved)} />
    </div>
  )
}

interface FrameMessage {
  type?: string
  message?: string
  text?: string
  href?: string
  data?: unknown
  wheel?: Pick<WheelEvent, 'clientX' | 'clientY' | 'deltaX' | 'deltaY' | 'deltaMode' | 'ctrlKey' | 'shiftKey'>
}

const MOST_DATA = 200_000

// An opaque-origin frame: agent code runs here without access to the app,
// talking back only through the artifact bridge's messages. Its document is
// rebuilt when its source changes; a change to the item's saved data reaches
// the running frame as a message instead.
function SandboxFrame({ id, title, source, data, build }: { id: string; title: string; source: string; data: unknown; build: (data: unknown) => string }) {
  const { prompt, commit } = useCanvas()
  const frame = useRef<HTMLIFrameElement>(null)
  const [error, setError] = useState('')
  const [built, setBuilt] = useState(() => ({ source, document: build(data) }))
  if (built.source !== source) {
    setBuilt({ source, document: build(data) })
    setError('')
  }
  const tell = useCallback(() => {
    frame.current?.contentWindow?.postMessage({ type: 'jaz:canvas-data', data: data ?? null }, '*')
  }, [data])
  useEffect(tell, [tell])
  useEffect(() => {
    const onMessage = (event: MessageEvent<FrameMessage>) => {
      if (event.source !== frame.current?.contentWindow) return
      const message = event.data
      if (message?.type === 'jaz:artifact-error') setError(message.message ?? 'Component error')
      if (message?.type === 'jaz:artifact-link' && message.href) window.open(message.href, '_blank', 'noopener,noreferrer')
      if (message?.type === 'jaz:artifact-send-prompt' && message.text?.trim()) prompt(message.text.trim())
      if (message?.type === 'jaz:canvas-data') {
        const saved = JSON.stringify(message.data ?? null)
        if (saved.length > MOST_DATA) setError('Saved data is too large')
        else if (saved !== JSON.stringify(data ?? null)) commit({ nodes: [{ id, data: message.data ?? null }] })
      }
      if (message?.type === 'jaz:canvas-wheel' && message.wheel) {
        const element = frame.current!
        const box = element.getBoundingClientRect()
        element.dispatchEvent(new WheelEvent('wheel', {
          ...message.wheel, bubbles: true, cancelable: true,
          clientX: box.x + message.wheel.clientX * box.width / element.clientWidth,
          clientY: box.y + message.wheel.clientY * box.height / element.clientHeight,
        }))
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [id, data, prompt, commit])
  return (
    <>
      <iframe ref={frame} className="canvas-frame" title={title} sandbox="allow-scripts allow-forms" referrerPolicy="no-referrer" srcDoc={built.document} onLoad={tell} />
      {error ? <div className="canvas-component-error overlay">{error}</div> : null}
    </>
  )
}
