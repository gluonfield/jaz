import { NodeToolbar, useReactFlow } from '@xyflow/react'
import { AlignCenterHorizontal, AlignCenterVertical, AlignEndHorizontal, AlignEndVertical, AlignStartHorizontal, AlignStartVertical, BringToFront, Copy, Lock, LockOpen, SendToBack, Trash2, type LucideIcon } from 'lucide-react'
import { ALIGNMENTS, alignItems, layerItems, type Alignment } from './arrange'
import { rectOfNode, type BoardEdge, type BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { ToolButton, hold } from './CanvasToolbar'

const ALIGN_ICONS: Record<Alignment, LucideIcon> = {
  left: AlignStartVertical,
  center: AlignCenterVertical,
  right: AlignEndVertical,
  top: AlignStartHorizontal,
  middle: AlignCenterHorizontal,
  bottom: AlignEndHorizontal,
}

// What the user does to whole items: lock, copy, restack, remove.
export function ArrangeControls({ ids }: { ids: string[] }) {
  const { view, commit, duplicate } = useCanvas()
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const picked = new Set(ids)
  const items = view.nodes.filter((item) => picked.has(item.id))
  const locked = items.length > 0 && items.every((item) => item.locked === true)
  const layer = (to: 'front' | 'back') => {
    const change = layerItems(view.nodes.filter((item) => item.type !== 'group'), picked, to)
    if (change.length) commit({ nodes: change })
  }
  return (
    <>
      <ToolButton label={locked ? 'Unlock' : 'Lock'} on={locked} onClick={() => commit({ nodes: ids.map((id) => ({ id, locked: locked ? null : true })) })}>
        {locked ? <Lock size={16} /> : <LockOpen size={16} />}
      </ToolButton>
      {locked ? null : (
        <>
          <ToolButton label="Duplicate" onClick={duplicate}><Copy size={16} /></ToolButton>
          <ToolButton label="Bring to front" onClick={() => layer('front')}><BringToFront size={16} /></ToolButton>
          <ToolButton label="Send to back" onClick={() => layer('back')}><SendToBack size={16} /></ToolButton>
          <ToolButton label="Delete" onClick={() => void flow.deleteElements({ nodes: ids.map((id) => ({ id })) })}><Trash2 size={16} /></ToolButton>
        </>
      )}
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
  return (
    <NodeToolbar nodeId={ids} isVisible className="canvas-toolbar nodrag nopan nowheel" offset={16} role="toolbar" aria-label="Selection actions" onMouseDown={hold}>
      {ALIGNMENTS.map((to) => {
        const Icon = ALIGN_ICONS[to]
        return <ToolButton key={to} label={`Align ${to}`} onClick={() => align(to)}><Icon size={16} /></ToolButton>
      })}
      <span className="canvas-toolbar-rule" />
      <ArrangeControls ids={ids} />
    </NodeToolbar>
  )
}
