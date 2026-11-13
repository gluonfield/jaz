import { Handle, NodeResizeControl, NodeResizer, Position, type NodeProps } from '@xyflow/react'
import { useRef, useState, type CSSProperties } from 'react'
import { MessageMarkdown } from '@/components/session/MessageMarkdown'
import type { CanvasItem } from '@/lib/api/canvas'
import { extension, flows, itemLabel, rounded, viewerFor, type BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { CSS_SHAPES, ShapeOutline } from './CanvasShapes'
import { RichEditor, SourceEditor, type TextEditorHandle } from './CanvasTextEditors'
import { TextToolbar } from './CanvasTextToolbar'
import { ItemToolbar } from './CanvasToolbar'
import { FileViewer } from './CanvasViewers'
import { FitContent } from '@/components/canvas/FitContent'
import { inkColor, lookOf, toggleList, toggleMark } from './format'
import { parseRich, toggleRichList, toggleRichMark, writeRich, type TextFormat } from './richtext'
import { ViewerFrame } from './ViewerFrame'

const SIDES = [
  ['top', Position.Top],
  ['right', Position.Right],
  ['bottom', Position.Bottom],
  ['left', Position.Left],
] as const

export function Handles() {
  return SIDES.map(([side, position]) => <Handle key={side} id={side} type="source" position={position} className="canvas-dot" />)
}

export function Resizer({ item, selected, minWidth = 120, minHeight = 60 }: { item: CanvasItem; selected: boolean; minWidth?: number; minHeight?: number }) {
  const { commit } = useCanvas()
  if (!selected || item.locked === true) return null
  // Text, tables and documents are as tall as their content, so only their width is
  // set by hand, which also lifts the height limit their author gave them.
  if (flows(item)) {
    return (['left', 'right'] as const).map((position) => (
      <NodeResizeControl
        key={position}
        position={position}
        resizeDirection="horizontal"
        minWidth={minWidth}
        className="canvas-resize-handle"
        onResizeEnd={(_, { x, width }) => commit({ nodes: [{ id: item.id, x: Math.round(x), width: Math.round(width), height: null }] })}
      />
    ))
  }
  return (
    <NodeResizer
      minWidth={minWidth}
      minHeight={minHeight}
      handleClassName="canvas-resize-handle"
      lineClassName="canvas-resize-line"
      onResizeEnd={(_, params) => commit({ nodes: [{ id: item.id, ...rounded(params) }] })}
    />
  )
}

export function Flash({ id }: { id: string }) {
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
      <Resizer item={data.item} selected={selected} />
      {selected ? <ItemToolbar label="Item" item={data.item} offset={34} /> : null}
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
      <Resizer item={data.item} selected={selected} />
      {selected ? <ItemToolbar label="Item" item={data.item} offset={34} /> : null}
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

export function TextNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { editing, setEditing, commit, connectSource } = useCanvas()
  const editor = useRef<TextEditorHandle>(null)
  const text = typeof data.item.text === 'string' ? data.item.text : ''
  const look = lookOf(data.item)
  const finish = (value: string) => {
    setEditing(null)
    if (!value.trim() && !text) {
      commit({ remove: [id] })
      return
    }
    if (value !== text) commit({ nodes: [{ id, text: value }] })
  }
  // A control formats what is selected in the editor, or else every line.
  const format = (kind: TextFormat) => {
    if (editing === id) {
      editor.current?.format(kind)
      return
    }
    const blocks = parseRich(text)
    const next = blocks
      ? writeRich(kind === 'list' || kind === 'numbered' ? toggleRichList(blocks, kind === 'list' ? 'ul' : 'ol') : toggleRichMark(blocks, kind))
      : kind === 'list' ? toggleList(text) : kind === 'bold' ? toggleMark(text, '**') : kind === 'italic' ? toggleMark(text, '*') : text
    if (next !== text) commit({ nodes: [{ id, text: next }] })
  }
  const base = look.kind === 'block' ? `canvas-block canvas-shape-${look.shape}` : look.kind === 'note' ? 'canvas-sticky' : 'canvas-text'
  const classes = [
    base,
    look.fill ? `canvas-filled${look.fill.name ? ` canvas-fill-${look.fill.name}` : ''}` : '',
    look.size ? `canvas-size-${look.size}` : '',
    look.align ? `canvas-align-${look.align}` : '',
    look.hollow ? 'hollow' : '',
    connectSource === id ? 'canvas-connect-source' : '',
  ].filter(Boolean).join(' ')
  const style = { '--canvas-fill': look.fill?.custom, '--canvas-ink': inkColor(look.ink) } as CSSProperties
  const rich = editing === id ? parseRich(text) : null
  const body = editing !== id
    ? <FitContent><MessageMarkdown text={text} /></FitContent>
    : rich
      ? <RichEditor ref={editor} blocks={rich} onDone={finish} />
      : <SourceEditor ref={editor} initial={text} onDone={finish} />
  return (
    <>
      <Resizer item={data.item} selected={selected && editing !== id} minWidth={look.kind === 'note' ? 120 : 40} minHeight={look.kind === 'note' ? 100 : 40} />
      {selected ? <TextToolbar item={data.item} onFormat={format} /> : null}
      <Handles />
      <div className={classes} style={style} onDoubleClick={() => data.item.locked !== true && setEditing(id)}>
        {look.shape && !CSS_SHAPES.includes(look.shape) ? <ShapeOutline shape={look.shape} /> : null}
        {look.kind === 'block' ? <div className="canvas-block-body">{body}</div> : body}
      </div>
      <Flash id={id} />
    </>
  )
}

export function GroupNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { dropTarget, commit } = useCanvas()
  const label = typeof data.item.label === 'string' && data.item.label ? data.item.label : 'Section'
  const [renaming, setRenaming] = useState(false)
  return (
    <>
      <Resizer item={data.item} selected={selected} minWidth={160} minHeight={120} />
      {selected ? <ItemToolbar label="Item" item={data.item} offset={34} /> : null}
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

export function LabelInput({ initial, onDone }: { initial: string; onDone: (value: string | null) => void }) {
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
