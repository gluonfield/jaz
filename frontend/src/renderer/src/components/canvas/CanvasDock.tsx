import { Hand, Hash, ImagePlus, MousePointer2, Pen, Smile, Spline, StickyNote, Table2, Type, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { ShapeOutline } from './CanvasShapes'
import { SHAPES, title, type Shape } from './format'

export type Tool = 'select' | 'hand' | 'sticky' | 'text' | 'shape' | 'table' | 'section' | 'connector' | 'pen' | 'sticker'

// Every button chooses a tool but the last, which opens the file picker. The
// shape tool, which also holds the table, draws itself.
const GROUPS: ({ tool: Exclude<Tool, 'shape' | 'table'> | 'upload'; label: string; key: string; Icon: LucideIcon } | 'shape')[][] = [
  [
    { tool: 'select', label: 'Select', key: 'V', Icon: MousePointer2 },
    { tool: 'hand', label: 'Hand', key: 'H', Icon: Hand },
  ],
  [
    { tool: 'sticky', label: 'Sticky note', key: 'S', Icon: StickyNote },
    { tool: 'text', label: 'Text', key: 'T', Icon: Type },
    'shape',
    { tool: 'section', label: 'Section', key: '⇧S', Icon: Hash },
    { tool: 'connector', label: 'Connector', key: 'X', Icon: Spline },
  ],
  [
    { tool: 'pen', label: 'Pen', key: 'P', Icon: Pen },
    { tool: 'sticker', label: 'Sticker', key: 'K', Icon: Smile },
    { tool: 'upload', label: 'Picture or file', key: 'I', Icon: ImagePlus },
  ],
]

// The shape tool shows the shape it puts down, and while the pointer is over
// it, every shape it could put down instead, and the table.
function ShapeTool({ tool, shape, onTool, onShape }: { tool: Tool; shape: Shape; onTool: (tool: Tool) => void; onShape: (shape: Shape) => void }) {
  const [open, setOpen] = useState(false)
  const pick = (choose: () => void) => () => {
    setOpen(false)
    choose()
  }
  return (
    <div className="canvas-dock-menu" onPointerEnter={() => setOpen(true)} onPointerLeave={() => setOpen(false)}>
      <button type="button" className={tool === 'shape' || tool === 'table' ? 'on' : ''} aria-label="Shape" aria-pressed={tool === 'shape'} onClick={pick(() => onTool('shape'))}>
        {tool === 'table' ? <Table2 size={18} strokeWidth={1.7} /> : <ShapeOutline shape={shape} />}
      </button>
      {open ? (
        <div className="canvas-dock-panel" role="menu" aria-label="Shapes">
          {SHAPES.map((kind) => (
            <button key={kind} type="button" role="menuitemradio" aria-checked={kind === shape} aria-label={title(kind)} onClick={pick(() => {
              onShape(kind)
              onTool('shape')
            })}>
              <ShapeOutline shape={kind} />
            </button>
          ))}
          <button type="button" role="menuitemradio" aria-checked={tool === 'table'} aria-label="Table" onClick={pick(() => onTool('table'))}>
            <Table2 size={18} strokeWidth={1.7} />
          </button>
        </div>
      ) : null}
    </div>
  )
}

export function CanvasDock({ tool, shape, onTool, onShape, onUpload }: { tool: Tool; shape: Shape; onTool: (tool: Tool) => void; onShape: (shape: Shape) => void; onUpload: () => void }) {
  return (
    <div className="canvas-dock canvas-overlay" role="toolbar" aria-label="Canvas tools">
      {GROUPS.map((group, i) => (
        <div key={i} className="canvas-dock-group">
          {group.map((entry) => entry === 'shape' ? <ShapeTool key={entry} tool={tool} shape={shape} onTool={onTool} onShape={onShape} /> : (
            <button
              key={entry.tool}
              type="button"
              className={tool === entry.tool ? 'on' : ''}
              aria-label={entry.label}
              aria-pressed={tool === entry.tool}
              data-tip={`${entry.label}  ${entry.key}`}
              onClick={() => (entry.tool === 'upload' ? onUpload() : onTool(entry.tool))}
            >
              <entry.Icon size={18} strokeWidth={1.7} />
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}
