import { Hand, Hash, ImagePlus, MousePointer2, Pen, Smile, Spline, StickyNote, Table2, Type, type LucideIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { ShapeOutline } from './CanvasShapes'
import { SHAPES, title, type Shape } from './format'

export type Tool = 'select' | 'hand' | 'sticky' | 'text' | 'shape' | 'table' | 'section' | 'connector' | 'pen' | 'sticker'

const TOOLS = {
  select: ['Select', 'V', MousePointer2],
  hand: ['Hand', 'H', Hand],
  sticky: ['Sticky note', 'S', StickyNote],
  text: ['Text', 'T', Type],
  section: ['Section', '⇧S', Hash],
  connector: ['Connector', 'X', Spline],
  pen: ['Pen', 'P', Pen],
  sticker: ['Sticker', 'K', Smile],
} satisfies Partial<Record<Tool, [label: string, key: string, Icon: LucideIcon]>>

function DockButton({ label, hint, on, onClick, children }: { label: string; hint?: string; on?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className={on ? 'on' : ''} aria-label={label} aria-pressed={on} data-tip={hint && `${label}  ${hint}`} onClick={onClick}>
      {children}
    </button>
  )
}

// The shape tool shows the shape it puts down, and while the pointer is over
// it, every shape it could put down instead, and the table. Its tip sits over
// that list, where the other tools show theirs over themselves.
function ShapeTool({ tool, shape, onTool, onShape }: { tool: Tool; shape: Shape; onTool: (tool: Tool) => void; onShape: (shape: Shape) => void }) {
  const [open, setOpen] = useState(false)
  const pick = (next: Tool, kind = shape) => () => {
    setOpen(false)
    onShape(kind)
    onTool(next)
  }
  return (
    <div className="canvas-dock-menu" onPointerEnter={() => setOpen(true)} onPointerLeave={() => setOpen(false)}>
      <DockButton label="Shapes" on={tool === 'shape' || tool === 'table'} onClick={pick('shape')}>
        {tool === 'table' ? <Table2 size={18} strokeWidth={1.7} /> : <ShapeOutline shape={shape} />}
      </DockButton>
      {open ? (
        <div className="canvas-dock-panel" role="menu" aria-label="Shapes" data-tip="Shapes  R">
          {SHAPES.map((kind) => (
            <button key={kind} type="button" role="menuitemradio" aria-checked={kind === shape} aria-label={title(kind)} onClick={pick('shape', kind)}>
              <ShapeOutline shape={kind} />
            </button>
          ))}
          <button type="button" role="menuitemradio" aria-checked={tool === 'table'} aria-label="Table" onClick={pick('table')}>
            <Table2 size={18} strokeWidth={1.7} />
          </button>
        </div>
      ) : null}
    </div>
  )
}

export function CanvasDock({ tool, shape, onTool, onShape, onUpload }: { tool: Tool; shape: Shape; onTool: (tool: Tool) => void; onShape: (shape: Shape) => void; onUpload: () => void }) {
  const button = (value: keyof typeof TOOLS) => {
    const [label, key, Icon] = TOOLS[value]
    return <DockButton label={label} hint={key} on={tool === value} onClick={() => onTool(value)}><Icon size={18} strokeWidth={1.7} /></DockButton>
  }
  return (
    <div className="canvas-dock canvas-overlay" role="toolbar" aria-label="Canvas tools">
      <div className="canvas-dock-group">
        {button('select')}
        {button('hand')}
      </div>
      <div className="canvas-dock-group">
        {button('sticky')}
        {button('text')}
        <ShapeTool tool={tool} shape={shape} onTool={onTool} onShape={onShape} />
        {button('section')}
        {button('connector')}
      </div>
      <div className="canvas-dock-group">
        {button('pen')}
        {button('sticker')}
        <DockButton label="Picture or file" hint="I" onClick={onUpload}><ImagePlus size={18} strokeWidth={1.7} /></DockButton>
      </div>
    </div>
  )
}
