import { NodeToolbar, useReactFlow } from '@xyflow/react'
import { AlignCenterHorizontal, AlignCenterVertical, AlignEndHorizontal, AlignEndVertical, AlignStartHorizontal, AlignStartVertical, BringToFront, ClipboardPaste, Copy, Ellipsis, Group, Lock, LockOpen, Paintbrush, SendToBack, Trash2, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { ALIGNMENTS, alignItems, layerItems, styleOf, type Alignment } from './arrange'
import { rectOfNode, sectionAround, type BoardEdge, type BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { Comments } from './CanvasComments'
import { Menu, Menus, ToolButton, hold } from './CanvasMenu'

type Act = [label: string, Icon: LucideIcon, act: () => void]

// The look last copied, with the kind of item it came from.
let style: { type: unknown; look: Partial<CanvasItem> } | null = null

// The controls of a selected item, floating above it at screen size. A locked
// item offers only its lock and its comments.
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
        <Comments item={item} />
        <ArrangeControls items={[item]} menu />
      </Menus>
    </NodeToolbar>
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
export function ArrangeControls({ items, menu }: { items: CanvasItem[]; menu?: boolean }) {
  const { commit, duplicate } = useCanvas()
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const locked = items.every((item) => item.locked === true)
  const layer = (to: 'front' | 'back') => {
    const change = layerItems(flow.getNodes().map((node) => node.data.item), new Set(items.map((item) => item.id)), to)
    if (change.length) commit({ nodes: change })
  }
  const alike = style ? items.filter((item) => item.type === style!.type) : []
  const copyStyle = () => {
    style = { type: items[0].type, look: styleOf(items[0]) }
  }
  const acts: Act[] = [
    ['Duplicate', Copy, duplicate],
    ...(items.length === 1 ? [['Copy style', Paintbrush, copyStyle] satisfies Act] : []),
    ...(alike.length ? [['Paste style', ClipboardPaste, () => commit({ nodes: alike.map((item) => ({ id: item.id, ...style!.look })) })] satisfies Act] : []),
    ['Bring to front', BringToFront, () => layer('front')],
    ['Send to back', SendToBack, () => layer('back')],
    ['Delete', Trash2, () => void flow.deleteElements({ nodes: items })],
  ]
  return (
    <>
      <ToolButton label={locked ? 'Unlock' : 'Lock'} on={locked} onClick={() => commit({ nodes: items.map(({ id }) => ({ id, locked: locked ? null : true })) })}>
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
export function SelectionToolbar({ nodes }: { nodes: BoardNode[] }) {
  const { commit } = useCanvas()
  const align = (to: Alignment) => {
    const free = nodes.filter((node) => node.data.item.locked !== true)
    commit({ nodes: alignItems(free.map((node) => ({ id: node.id, ...rectOfNode(node) })), to) })
  }
  return (
    <NodeToolbar nodeId={nodes.map((node) => node.id)} isVisible className="canvas-toolbar nodrag nopan nowheel" offset={16} role="toolbar" aria-label="Selection actions" onMouseDown={hold}>
      {ALIGNMENTS.map((to) => {
        const Icon = ALIGN_ICONS[to]
        return <ToolButton key={to} label={`Align ${to}`} onClick={() => align(to)}><Icon size={16} /></ToolButton>
      })}
      <span className="canvas-toolbar-rule" />
      <ToolButton label="Group" onClick={() => commit({ nodes: [sectionAround(nodes)] })}><Group size={16} /></ToolButton>
      <ArrangeControls items={nodes.map((node) => node.data.item)} />
    </NodeToolbar>
  )
}
