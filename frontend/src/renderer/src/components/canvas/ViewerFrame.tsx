import type { ReactNode } from 'react'
import type { CanvasFile } from '@/lib/api/canvas'
import { sessionFileRawUrl } from '@/lib/api/sessions'
import { useCanvas } from './canvasContext'

export function rawUrl(sessionId: string, file: CanvasFile, cloudRunId?: string): string {
  const base = sessionFileRawUrl(sessionId, file.path, cloudRunId)
  const sep = base.includes('?') ? '&' : '?'
  return `${base}${sep}v=${encodeURIComponent(file.version ?? '')}`
}

export function ViewerFrame({ id, status, focusable, controls, paper, children }: {
  id: string
  status?: ReactNode
  focusable?: boolean
  controls?: ReactNode
  paper?: boolean
  children: ReactNode
}) {
  const { focused, setFocused } = useCanvas()
  const live = focused === id
  return (
    <div className="canvas-viewer">
      <div className={`canvas-viewer-body${focusable && !live ? ' inert' : ''}${live ? ' nodrag nowheel nopan' : ''}${paper ? ' paper' : ''}`}>{children}</div>
      {focusable ? (
        <div className="canvas-viewer-toolbar nodrag nowheel nopan">
          {status ? <span className="canvas-viewer-status">{status}</span> : null}
          {live && controls ? <span className="canvas-viewer-controls">{controls}</span> : null}
          <button type="button" className={`canvas-focus nodrag${live ? ' on' : ''}`} onClick={() => setFocused(live ? null : id)}>
            {live ? 'Done' : 'Focus'}
          </button>
        </div>
      ) : null}
    </div>
  )
}
