import { NodeToolbar, useReactFlow } from '@xyflow/react'
import { AlignCenterHorizontal, AlignCenterVertical, AlignEndHorizontal, AlignEndVertical, AlignStartHorizontal, AlignStartVertical, BringToFront, ClipboardPaste, Copy, Ellipsis, Group, Lock, LockOpen, Paintbrush, SendToBack, Trash2, type LucideIcon } from 'lucide-react'
import { createContext, use, useState, type MouseEvent, type ReactNode } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { ALIGNMENTS, alignItems, layerItems, styleOf, type Alignment } from './arrange'
import { bounds, newId, padded, rectOfNode, rounded, type BoardEdge, type BoardNode } from './board'
import { useCanvas } from './canvasContext'

// Pressing a control must not take the focus from the text being edited.
export const hold = (event: MouseEvent) => event.preventDefault()

// The look last copied, with the kind of item it came from.
let style: { type: unknown; look: Partial<CanvasItem> } | null = null

// The label of the one open menu.
const OpenMenu = createContext<[string | null, (label: string | null) => void]>([null, () => {}])

// The controls of a selected item, floating above it at screen size. A locked
// item offers only its lock.
export function Menus({ children }: { children: ReactNode }) {
  return <OpenMenu value={useState<string | null>(null)}>{children}</OpenMenu>
}

export function ItemToolbar({ label, item, offset = 16, children }: { label: string; item: CanvasItem; offset?: number; children?: ReactNode }) {
  return (
    <NodeToolbar className="canvas-toolbar nodrag nopan nowheel" offset={offset} role="toolbar" aria-label={label} onMouseDown={hold}>
      <Menus>
        {item.locked === true || !children ? null : (
          <>
            {children}
            <span className="canvas-toolbar-rule" />
          </>
        )}
        <ArrangeControls ids={[item.id]} menu />
      </Menus>
    </NodeToolbar>
  )
}

export function ToolButton({ label, on, onClick, children }: { label: string; on?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className={on ? 'on' : ''} aria-label={label} aria-pressed={on} title={label} onClick={onClick}>
      {children}
    </button>
  )
}

export function Menu({ label, face, children }: { label: string; face: ReactNode; children: ReactNode }) {
  const [current, setCurrent] = use(OpenMenu)
  const open = current === label
  return (
    <div className="canvas-toolbar-menu">
      <button type="button" className={open ? 'on' : ''} aria-label={label} aria-expanded={open} title={label} onClick={() => setCurrent(open ? null : label)}>
        {face}
      </button>
      {open ? <div className="canvas-toolbar-panel" role="menu" aria-label={label} onClick={() => setCurrent(null)}>{children}</div> : null}
    </div>
  )
}

const ALIGN_ICONS: Record<Alignment, LucideIcon> = {
  left: AlignStartVertical,
  center: AlignCenterVertical,
  right: AlignEndVertical,
  top: AlignStartHorizontal,
  middle: AlignCenterHorizontal,
  bottom: AlignEndHorizontal,
}

// What the user does to whole items: lock, copy, restack, remove.
export function ArrangeControls({ ids, menu }: { ids: string[]; menu?: boolean }) {
  const { view, commit, duplicate } = useCanvas()
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const picked = new Set(ids)
  const items = view.nodes.filter((item) => picked.has(item.id))
  const locked = items.length > 0 && items.every((item) => item.locked === true)
  const layer = (to: 'front' | 'back') => {
    const change = layerItems(view.nodes.filter((item) => item.type !== 'group'), picked, to)
    if (change.length) commit({ nodes: change })
  }
  const alike = style ? items.filter((item) => item.type === style!.type) : []
  const acts: [string, LucideIcon, () => void][] = [
    ['Duplicate', Copy, duplicate],
    ...(items.length === 1 ? [['Copy style', Paintbrush, () => {
      style = { type: items[0].type, look: styleOf(items[0]) }
    }] as [string, LucideIcon, () => void]] : []),
    ...(alike.length ? [['Paste style', ClipboardPaste, () => commit({ nodes: alike.map((item) => ({ id: item.id, ...style!.look })) })] as [string, LucideIcon, () => void]] : []),
    ['Bring to front', BringToFront, () => layer('front')],
    ['Send to back', SendToBack, () => layer('back')],
    ['Delete', Trash2, () => void flow.deleteElements({ nodes: ids.map((id) => ({ id })) })],
  ]
  return (
    <>
      <ToolButton label={locked ? 'Unlock' : 'Lock'} on={locked} onClick={() => commit({ nodes: ids.map((id) => ({ id, locked: locked ? null : true })) })}>
        {locked ? <Lock size={16} /> : <LockOpen size={16} />}
      </ToolButton>
      {locked ? null : menu ? <Menu label="More" face={<Ellipsis size={16} />}>{acts.map(([label, Icon, act]) => (
        <button key={label} type="button" role="menuitem" className="canvas-toolbar-row" onClick={act}><Icon size={14} />{label}</button>
      ))}</Menu> : acts.map(([label, Icon, act]) => (
        <ToolButton key={label} label={label} onClick={act}><Icon size={16} /></ToolButton>
      ))}
    </>
  )
}

// The controls of several selected items, above the box around them.
export function SelectionToolbar({ ids }: { ids: string[] }) {
  const { commit } = useCanvas()
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const align = (to: Alignment) => {
    const free = flow.getNodes().filter((node) => ids.includes(node.id) && node.data.item.locked !== true)
    commit({ nodes: alignItems(free.map((node) => ({ id: node.id, ...rectOfNode(node) })), to) })
  }
  const group = () => {
    const box = padded(bounds(flow.getNodes().filter((node) => ids.includes(node.id)).map(rectOfNode)))
    commit({ nodes: [{ id: newId('section'), type: 'group', label: 'Section', ...rounded(box) }] })
  }
  return (
    <NodeToolbar nodeId={ids} isVisible className="canvas-toolbar nodrag nopan nowheel" offset={16} role="toolbar" aria-label="Selection actions" onMouseDown={hold}>
      {ALIGNMENTS.map((to) => {
        const Icon = ALIGN_ICONS[to]
        return <ToolButton key={to} label={`Align ${to}`} onClick={() => align(to)}><Icon size={16} /></ToolButton>
      })}
      <span className="canvas-toolbar-rule" />
      <ToolButton label="Group" onClick={group}><Group size={16} /></ToolButton>
      <ArrangeControls ids={ids} />
    </NodeToolbar>
  )
}
