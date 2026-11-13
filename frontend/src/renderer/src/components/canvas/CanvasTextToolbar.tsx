import { useReactFlow } from '@xyflow/react'
import { Bold, Code, Italic, List, ListChecks, ListOrdered, StickyNote, Strikethrough, TextAlignCenter, TextAlignEnd, TextAlignStart, Type, type LucideIcon } from 'lucide-react'
import type { CSSProperties } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { useCanvas } from './canvasContext'
import { ShapeOutline } from './CanvasShapes'
import { ItemToolbar, Menu, ToolButton } from './CanvasToolbar'
import { ALIGNS, FILLS, INKS, SHAPES, SIZES, inkColor, lookOf, type Tint } from './format'
import type { TextFormat } from './richtext'

const ALIGN_ICONS = { left: TextAlignStart, center: TextAlignCenter, right: TextAlignEnd }
const FORMATS: [TextFormat, string, LucideIcon][] = [
  ['bold', 'Bold', Bold],
  ['italic', 'Italic', Italic],
  ['strike', 'Strikethrough', Strikethrough],
  ['code', 'Code', Code],
  ['list', 'Bulleted list', List],
  ['numbered', 'Numbered list', ListOrdered],
  ['checklist', 'Checklist', ListChecks],
]
const title = (word: string) => word[0].toUpperCase() + word.slice(1)

export function swatch(kind: 'fill' | 'ink', tint: Tint | undefined): { className: string; style: CSSProperties } {
  return {
    className: `canvas-swatch${kind === 'fill' && tint?.name ? ` canvas-fill-${tint.name}` : ''}${tint ? '' : ' none'}`,
    style: { [`--canvas-${kind}`]: kind === 'ink' ? inkColor(tint) : tint?.custom } as CSSProperties,
  }
}

export function SizeMenu({ item }: { item: CanvasItem }) {
  const { commit } = useCanvas()
  const size = lookOf(item).size ?? 'small'
  return (
    <Menu label="Text size" face={<span className="canvas-toolbar-word">{title(size)}</span>}>
      {SIZES.map((option) => (
        <button key={option} type="button" role="menuitemradio" aria-checked={option === size} className="canvas-toolbar-row" onClick={() => commit({ nodes: [{ id: item.id, size: option }] })}>
          {title(option)}
        </button>
      ))}
    </Menu>
  )
}

export function TextToolbar({ item, onFormat }: { item: CanvasItem; onFormat: (format: TextFormat) => void }) {
  const { commit } = useCanvas()
  const flow = useReactFlow()
  const look = lookOf(item)
  const set = (fields: Partial<CanvasItem>) => commit({ nodes: [{ id: item.id, ...fields }] })
  // Plain text has no height of its own, so a block starts at the size it shows.
  const boxed = () => (typeof item.height === 'number' ? {} : { height: Math.max(80, Math.round((flow.getNode(item.id)?.measured?.height ?? 0) + 32)) })
  const align = look.align ?? (look.kind === 'block' ? 'center' : 'left')
  const AlignIcon = ALIGN_ICONS[align]
  const plain = look.kind === 'text'
  return (
    <ItemToolbar label="Format" item={item}>
      <Menu label="Shape" face={look.shape ? <ShapeOutline shape={look.shape} /> : plain ? <Type size={16} /> : <StickyNote size={16} />}>
        <div className="canvas-toolbar-grid">
          <button type="button" role="menuitemradio" aria-checked={plain} aria-label="Text" title="Text" onClick={() => set({ shape: null, color: null, hollow: null, height: null })}>
            <Type size={16} />
          </button>
          <button type="button" role="menuitemradio" aria-checked={look.kind === 'note'} aria-label="Note" title="Note" onClick={() => set({ shape: null, hollow: null, color: item.color ?? 'yellow', ...boxed() })}>
            <StickyNote size={16} />
          </button>
          {SHAPES.map((shape) => (
            <button key={shape} type="button" role="menuitemradio" aria-checked={shape === look.shape} aria-label={title(shape)} title={title(shape)} onClick={() => set({ shape, ...boxed() })}>
              <ShapeOutline shape={shape} />
            </button>
          ))}
        </div>
      </Menu>
      <span className="canvas-toolbar-rule" />
      <SizeMenu item={item} />
      {FORMATS.map(([kind, label, Icon]) => <ToolButton key={kind} label={label} onClick={() => onFormat(kind)}><Icon size={16} /></ToolButton>)}
      <ToolButton label={`Align ${align}`} onClick={() => set({ textAlign: ALIGNS[(ALIGNS.indexOf(align) + 1) % ALIGNS.length] })}><AlignIcon size={16} /></ToolButton>
      <span className="canvas-toolbar-rule" />
      <Menu label={plain ? 'Text colour' : 'Fill'} face={<span {...swatch(plain ? 'ink' : 'fill', plain ? look.ink : look.fill)} />}>
        <div className="canvas-toolbar-swatches">
          {look.kind === 'note' ? null : (
            <button type="button" role="menuitemradio" aria-checked={!(plain ? look.ink : look.fill)} aria-label="Default" title="Default" onClick={() => set(plain ? { ink: null } : { color: null })}>
              <span {...swatch('fill', undefined)} />
            </button>
          )}
          {(plain ? INKS : FILLS).map((name) => (
            <button key={name} type="button" role="menuitemradio" aria-checked={(plain ? look.ink : look.fill)?.name === name} aria-label={title(name)} title={title(name)} onClick={() => set(plain ? { ink: name } : { color: name })}>
              <span {...swatch(plain ? 'ink' : 'fill', { name })} />
            </button>
          ))}
        </div>
        {look.kind === 'block' ? (
          <button type="button" role="menuitemcheckbox" aria-checked={look.hollow} className="canvas-toolbar-row" onClick={() => set({ hollow: look.hollow ? null : true })}>
            Outline only
          </button>
        ) : null}
      </Menu>
    </ItemToolbar>
  )
}
