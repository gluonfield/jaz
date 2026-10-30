import {
  BaseEdge,
  EdgeLabelRenderer,
  Handle,
  NodeResizer,
  Position,
  getBezierPath,
  type EdgeProps,
  type NodeProps,
} from '@xyflow/react'
import { useEffect, useRef, useState } from 'react'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import { extension, itemLabel, rounded, viewerFor, type BoardEdge, type BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { FileViewer } from './CanvasViewers'
import { FitContent } from '@/components/canvas/FitContent'
import { ViewerFrame } from './ViewerFrame'

const SIDES = [
  ['top', Position.Top],
  ['right', Position.Right],
  ['bottom', Position.Bottom],
  ['left', Position.Left],
] as const

function Handles() {
  return SIDES.map(([side, position]) => <Handle key={side} id={side} type="source" position={position} className="canvas-dot" />)
}

function Resizer({ id, selected, minWidth = 120, minHeight = 60 }: { id: string; selected: boolean; minWidth?: number; minHeight?: number }) {
  const { commit } = useCanvas()
  return (
    <NodeResizer
      isVisible={selected}
      minWidth={minWidth}
      minHeight={minHeight}
      handleClassName="canvas-resize-handle"
      lineClassName="canvas-resize-line"
      onResizeEnd={(_, params) => commit({ nodes: [{ id, ...rounded(params) }] })}
    />
  )
}

function Flash({ id }: { id: string }) {
  const { flash } = useCanvas()
  return flash[id] ? <div key={flash[id]} className="canvas-flash" /> : null
}

function ItemLabel({ badge, name }: { badge: string; name: string }) {
  return (
    <div className="canvas-item-label">
      <span className="canvas-badge">{badge}</span>
      <span className="truncate">{name}</span>
    </div>
  )
}

// Sandboxed content takes the pointer as soon as it is selected; until then a
// shield keeps clicks and drags on the canvas.
function shielded(path: string): boolean {
  return ['component', 'html'].includes(viewerFor(path))
}

export function FileNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { view, connectSource } = useCanvas()
  const path = typeof data.item.file === 'string' ? data.item.file : ''
  const file = view.files[id] ?? { path }
  return (
    <>
      <Resizer id={id} selected={selected} />
      <Handles />
      <ItemLabel badge={extension(path).toUpperCase() || 'FILE'} name={itemLabel(data.item)} />
      <div className={`canvas-card${connectSource === id ? ' canvas-connect-source' : ''}`}>
        <FileViewer id={id} file={file} />
        {shielded(path) && !selected ? <div className="canvas-shield" /> : null}
        <Flash id={id} />
      </div>
    </>
  )
}

export function LinkNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { connectSource } = useCanvas()
  const url = typeof data.item.url === 'string' ? data.item.url : ''
  const host = URL.canParse(url) ? new URL(url).host : url
  return (
    <>
      <Resizer id={id} selected={selected} />
      <Handles />
      <ItemLabel badge="WEB" name={host} />
      <div className={`canvas-card${connectSource === id ? ' canvas-connect-source' : ''}`}>
        <ViewerFrame id={id} focusable>
          <iframe className="canvas-frame" title={host} src={url} sandbox="allow-scripts allow-same-origin allow-forms allow-popups" allow="autoplay; encrypted-media; fullscreen; picture-in-picture" />
        </ViewerFrame>
        <Flash id={id} />
      </div>
    </>
  )
}

const STICKY_COLORS = new Set(['1', '2', '3', '4', '5', '6'])

export function TextNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { editing, setEditing, commit, connectSource } = useCanvas()
  const text = typeof data.item.text === 'string' ? data.item.text : ''
  const color = typeof data.item.color === 'string' ? data.item.color : ''
  const sticky = color !== ''
  const finish = (value: string) => {
    setEditing(null)
    if (!value.trim() && !text) {
      commit({ remove: [id] })
      return
    }
    if (value !== text) commit({ nodes: [{ id, text: value }] })
  }
  const style = sticky && !STICKY_COLORS.has(color) ? { background: color } : undefined
  return (
    <>
      <Resizer id={id} selected={selected && editing !== id} minWidth={sticky ? 120 : 40} minHeight={sticky ? 100 : 24} />
      <Handles />
      <div
        className={`${sticky ? `canvas-sticky canvas-sticky-${STICKY_COLORS.has(color) ? color : 'custom'}` : 'canvas-text'}${connectSource === id ? ' canvas-connect-source' : ''}`}
        style={style}
        onDoubleClick={() => setEditing(id)}
      >
        {editing === id ? <TextEditor initial={text} onDone={finish} /> : <FitContent><MessageMarkdown text={text} /></FitContent>}
      </div>
      <Flash id={id} />
    </>
  )
}

function TextEditor({ initial, onDone }: { initial: string; onDone: (value: string) => void }) {
  const [value, setValue] = useState(initial)
  const field = useRef<HTMLTextAreaElement>(null)
  // React Flow mounts a new node hidden until it is measured, so focus waits
  // for it to become visible.
  useEffect(() => {
    let frame = 0
    let tries = 0
    const focus = () => {
      field.current?.focus()
      if (document.activeElement !== field.current && tries++ < 30) frame = requestAnimationFrame(focus)
    }
    frame = requestAnimationFrame(focus)
    return () => cancelAnimationFrame(frame)
  }, [])
  return (
    <textarea
      ref={field}
      className="canvas-text-editor nodrag nopan"
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => onDone(value)}
      onKeyDown={(event) => {
        if (event.key === 'Escape') event.currentTarget.blur()
      }}
    />
  )
}

export function GroupNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { dropTarget, commit } = useCanvas()
  const label = typeof data.item.label === 'string' && data.item.label ? data.item.label : 'Section'
  const [renaming, setRenaming] = useState(false)
  return (
    <>
      <Resizer id={id} selected={selected} minWidth={160} minHeight={120} />
      <div
        className="canvas-section-label"
        onDoubleClick={(event) => {
          event.stopPropagation()
          setRenaming(true)
        }}
      >
        {renaming ? (
          <LabelInput
            initial={label}
            onDone={(value) => {
              setRenaming(false)
              if (value?.trim() && value.trim() !== label) commit({ nodes: [{ id, label: value.trim() }] })
            }}
          />
        ) : label}
      </div>
      <div className={`canvas-section${dropTarget === id ? ' target' : ''}`}>
        <Flash id={id} />
      </div>
    </>
  )
}

function LabelInput({ initial, onDone }: { initial: string; onDone: (value: string | null) => void }) {
  const [value, setValue] = useState(initial)
  return (
    <input
      autoFocus
      className="nodrag"
      value={value}
      placeholder="Label"
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => onDone(value)}
      onKeyDown={(event) => {
        if (event.key === 'Enter') onDone(value)
        if (event.key === 'Escape') onDone(null)
      }}
      style={{ width: `${Math.max(5, value.length + 1)}ch` }}
    />
  )
}

export function ConnectorEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, markerEnd, markerStart, selected }: EdgeProps<BoardEdge>) {
  const { editing, setEditing, commit } = useCanvas()
  const [path, labelX, labelY] = getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition })
  return (
    <>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} markerStart={markerStart} interactionWidth={18} className={selected ? 'canvas-edge selected' : 'canvas-edge'} />
      {data?.label || editing === id ? (
        <EdgeLabelRenderer>
          <div
            className={`canvas-edge-label nodrag nopan${selected ? ' selected' : ''}`}
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            onDoubleClick={() => setEditing(id)}
          >
            {editing === id ? (
              <LabelInput
                initial={data?.label ?? ''}
                onDone={(value) => {
                  setEditing(null)
                  if (value !== null && value.trim() !== (data?.label ?? '')) commit({ edges: [{ id, label: value.trim() || null }] })
                }}
              />
            ) : data?.label}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  )
}
