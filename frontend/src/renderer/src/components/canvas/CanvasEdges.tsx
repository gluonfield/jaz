import { BaseEdge, EdgeLabelRenderer, ViewportPortal, getBezierPath, getSmoothStepPath, getStraightPath, useReactFlow, type EdgeProps } from '@xyflow/react'
import { ArrowLeft, ArrowRight, Circle, CornerDownRight, Minus, Spline, Trash2, type LucideIcon } from 'lucide-react'
import type { CSSProperties } from 'react'
import type { BoardEdge } from './board'
import { useCanvas } from './canvasContext'
import { LabelInput } from './CanvasNodes'
import { swatch } from './CanvasTextToolbar'
import { Menu, Menus, ToolButton, hold } from './CanvasMenu'
import { DASHES, ENDS, INKS, ROUTES, inkColor, type EdgeLook } from './format'

const PATHS = { curve: getBezierPath, elbow: getSmoothStepPath, straight: getStraightPath }
const DASH_ARRAYS = { solid: undefined, dashed: '8 6', dotted: '1.5 5' }
const ROUTE_ICONS: Record<EdgeLook['route'], LucideIcon> = { curve: Spline, elbow: CornerDownRight, straight: Minus }
const END_ICONS: Record<'fromEnd' | 'toEnd', Record<EdgeLook['toEnd'], LucideIcon>> = {
  fromEnd: { none: Minus, arrow: ArrowLeft, circle: Circle },
  toEnd: { none: Minus, arrow: ArrowRight, circle: Circle },
}
const after = <T,>(options: readonly T[], value: T) => options[(options.indexOf(value) + 1) % options.length]
const title = (word: string) => word[0].toUpperCase() + word.slice(1)

export function ConnectorEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, markerEnd, markerStart, selected }: EdgeProps<BoardEdge>) {
  const { editing, setEditing, commit } = useCanvas()
  const look = data!.look
  const [path, labelX, labelY] = PATHS[look.route]({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition })
  const style = { '--canvas-edge': inkColor(look.ink), strokeDasharray: DASH_ARRAYS[look.dash], strokeLinecap: look.dash === 'dotted' ? 'round' : undefined } as CSSProperties
  const dots = [look.fromEnd === 'circle' ? [sourceX, sourceY] : null, look.toEnd === 'circle' ? [targetX, targetY] : null]
  return (
    <>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} markerStart={markerStart} interactionWidth={18} className={selected ? 'canvas-edge selected' : 'canvas-edge'} style={style} />
      {dots.map((dot, index) => (dot ? <circle key={index} className={selected ? 'canvas-edge canvas-edge-dot selected' : 'canvas-edge canvas-edge-dot'} style={style} cx={dot[0]} cy={dot[1]} r={4} /> : null))}
      <EdgeLabelRenderer>
        {data?.label || editing === id ? (
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
        ) : null}
      </EdgeLabelRenderer>
      {selected ? <ViewportPortal><EdgeToolbar id={id} look={look} x={labelX} y={labelY} /></ViewportPortal> : null}
    </>
  )
}

// The controls of a selected connector, above its middle at screen size.
function EdgeToolbar({ id, look, x, y }: { id: string; look: EdgeLook; x: number; y: number }) {
  const { commit } = useCanvas()
  const flow = useReactFlow()
  const set = (fields: Record<string, string | null>) => commit({ edges: [{ id, ...fields }] })
  const RouteIcon = ROUTE_ICONS[look.route]
  return (
    <div className="canvas-toolbar canvas-edge-toolbar nodrag nopan nowheel" role="toolbar" aria-label="Connector" style={{ left: x, top: y }} onMouseDown={hold}>
      <Menus>
        <ToolButton label={`Route: ${look.route}`} onClick={() => set({ route: after(ROUTES, look.route) })}><RouteIcon size={16} /></ToolButton>
        <ToolButton label={`Line: ${look.dash}`} onClick={() => set({ dash: after(DASHES, look.dash) })}>
          <svg width="18" height="16" aria-hidden><path d="M1 8H17" stroke="currentColor" strokeWidth="1.8" strokeDasharray={DASH_ARRAYS[look.dash]} strokeLinecap="round" /></svg>
        </ToolButton>
        {(['fromEnd', 'toEnd'] as const).map((end) => {
          const Icon = END_ICONS[end][look[end]]
          return <ToolButton key={end} label={`${end === 'fromEnd' ? 'Start' : 'End'}: ${look[end]}`} onClick={() => set({ [end]: after(ENDS, look[end]) })}><Icon size={look[end] === 'circle' ? 11 : 16} /></ToolButton>
        })}
        <span className="canvas-toolbar-rule" />
        <Menu label="Line colour" face={<span {...swatch('ink', look.ink)} />}>
          <div className="canvas-toolbar-swatches">
            <button type="button" role="menuitemradio" aria-checked={!look.ink} aria-label="Default" title="Default" onClick={() => set({ color: null })}><span {...swatch('ink', undefined)} /></button>
            {INKS.map((name) => (
              <button key={name} type="button" role="menuitemradio" aria-checked={look.ink?.name === name} aria-label={title(name)} title={title(name)} onClick={() => set({ color: name })}><span {...swatch('ink', { name })} /></button>
            ))}
          </div>
        </Menu>
        <ToolButton label="Delete" onClick={() => void flow.deleteElements({ edges: [{ id }] })}><Trash2 size={16} /></ToolButton>
      </Menus>
    </div>
  )
}
