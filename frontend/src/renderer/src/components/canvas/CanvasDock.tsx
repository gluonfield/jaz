import { Eraser, Hand, Hash, MousePointer2, Pen, Spline, Square, StickyNote, Table2, Type, type LucideIcon } from 'lucide-react'

export type Tool = 'select' | 'hand' | 'sticky' | 'text' | 'shape' | 'table' | 'section' | 'connector' | 'pen' | 'eraser'

const GROUPS: { tool: Tool; label: string; key: string; Icon: LucideIcon }[][] = [
  [
    { tool: 'select', label: 'Select', key: 'V', Icon: MousePointer2 },
    { tool: 'hand', label: 'Hand', key: 'H', Icon: Hand },
  ],
  [
    { tool: 'sticky', label: 'Sticky note', key: 'S', Icon: StickyNote },
    { tool: 'text', label: 'Text', key: 'T', Icon: Type },
    { tool: 'shape', label: 'Shape', key: 'R', Icon: Square },
    { tool: 'table', label: 'Table', key: 'B', Icon: Table2 },
    { tool: 'section', label: 'Section', key: '⇧S', Icon: Hash },
    { tool: 'connector', label: 'Connector', key: 'X', Icon: Spline },
  ],
  [
    { tool: 'pen', label: 'Pen', key: 'P', Icon: Pen },
    { tool: 'eraser', label: 'Eraser', key: 'E', Icon: Eraser },
  ],
]

export function CanvasDock({ tool, onTool }: { tool: Tool; onTool: (tool: Tool) => void }) {
  return (
    <div className="canvas-dock canvas-overlay" role="toolbar" aria-label="Canvas tools">
      {GROUPS.map((group, i) => (
        <div key={i} className="canvas-dock-group">
          {group.map(({ tool: value, label, key, Icon }) => (
            <button
              key={value}
              type="button"
              className={tool === value ? 'on' : ''}
              aria-label={label}
              aria-pressed={tool === value}
              data-tip={`${label}  ${key}`}
              onClick={() => onTool(value)}
            >
              <Icon size={18} strokeWidth={1.7} />
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}
