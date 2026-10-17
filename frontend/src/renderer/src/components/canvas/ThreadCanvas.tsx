import {
  Background,
  BackgroundVariant,
  ConnectionMode,
  MiniMap,
  Panel,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  useViewport,
  type Connection,
  type OnSelectionChangeParams,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import './canvas.css'
import { Minus, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react'
import type { CanvasItem, CanvasPatch, CanvasView } from '@/lib/api/canvas'
import { useArtifactThemeCSS } from '@/lib/hooks/useArtifactThemeCSS'
import {
  bounds,
  contains,
  fileName,
  focusable,
  growToFit,
  innermostSection,
  itemLabel,
  mergeEdges,
  mergeNodes,
  newId,
  rectOfNode,
  padded,
  rounded,
  sectionContents,
  sidesBetween,
  toFlowEdges,
  toFlowNodes,
  viewerFor,
  withContents,
  type BoardEdge,
  type BoardNode,
  type Rect,
} from './board'
import { CanvasContext, type CanvasBoard } from './canvasContext'
import { CanvasDock, type Tool } from './CanvasDock'
import { ConnectorEdge, FileNode, GroupNode, LinkNode, TextNode } from './CanvasNodes'
import { fitPadding, useAgentFollow } from './useAgentFollow'
import type { CanvasPick } from './useCanvasContexts'
import { useCanvasInput, type Bindings } from './useCanvasInput'
import { useCanvasSync } from './useCanvasSync'
import { useSectionDrag } from './useSectionDrag'

interface Props {
  sessionId: string
  // Width of the board's left edge that the chat covers.
  inset: number
  working: boolean
  selection: string[]
  onSelect: (picks: CanvasPick[]) => void
  onPrompt: (text: string) => void
}

const nodeTypes = { text: TextNode, file: FileNode, link: LinkNode, group: GroupNode }
const edgeTypes = { connector: ConnectorEdge }
const TOOL_KEYS: Record<string, Tool> = { V: 'select', H: 'hand', S: 'sticky', T: 'text', X: 'connector' }
const HINTS: Record<string, string> = { link: 'Use the page', cad: 'Drag to rotate, scroll to zoom', dxf: 'Drag to pan, scroll to zoom', pdf: 'Scroll and use the page controls' }

export function ThreadCanvas(props: Props) {
  const sync = useCanvasSync(props.sessionId, props.working)
  if (!sync.view || !sync.local) {
    return <div className="canvas">{sync.error ? <div className="canvas-overlay canvas-banner danger">{sync.error.message}</div> : null}</div>
  }
  return (
    <ReactFlowProvider>
      <Board {...props} {...sync} view={sync.view} local={sync.local} />
    </ReactFlowProvider>
  )
}

interface BoardProps extends Props {
  view: CanvasView
  local: CanvasView
  commit: (change: CanvasPatch) => void
  replay: (from: 'undo' | 'redo') => void
  error: Error | null
}

function Board({ sessionId, inset, selection, onSelect, onPrompt, view, local, commit, replay, error }: BoardProps) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { zoom } = useViewport()
  const theme = useArtifactThemeCSS()
  const container = useRef<HTMLDivElement>(null)
  const { flash, news, show, touch } = useAgentFollow(view, local, container, inset)
  const [nodes, setNodes, onNodesChange] = useNodesState<BoardNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<BoardEdge>([])
  const { dragging, dropTarget, ...drag } = useSectionDrag(setNodes, commit)
  // Nothing from the server lands mid-drag; the drag's own result merges after.
  useEffect(() => {
    if (!dragging) {
      setNodes((current) => mergeNodes(current, toFlowNodes(local), new Set(selection)))
    }
  }, [local, selection, dragging, setNodes])
  useEffect(() => {
    if (!dragging) {
      setEdges((current) => mergeEdges(current, toFlowEdges(local)))
    }
  }, [local, dragging, setEdges])

  const [tool, setTool] = useState<Tool>('select')
  const [editing, setEditing] = useState<string | null>(null)
  const [focused, setFocused] = useState<string | null>(null)
  const [connectSource, setConnectSource] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState<{ x0: number; y0: number; x1: number; y1: number } | null>(null)

  const onSelectionChange = useCallback(({ nodes: picked }: OnSelectionChangeParams<BoardNode, BoardEdge>) => {
    onSelect(picked.map((node) => ({ node: node.id, label: itemLabel(node.data.item) })))
  }, [onSelect])

  // A new item dropped inside a section grows the section around it.
  const sectionFor = (rect: Rect): Partial<CanvasItem>[] => {
    const sections = flow.getNodes().filter((node) => node.type === 'group').map((node) => ({ id: node.id, rect: rectOfNode(node) }))
    const section = innermostSection(sections, rect.x + rect.width / 2, rect.y + rect.height / 2)
    if (!section || contains(section.rect, rect)) return []
    return [{ id: section.id, ...rounded(growToFit(section.rect, [rect])) }]
  }

  const create = (kind: Tool, clientX: number, clientY: number) => {
    const at = flow.screenToFlowPosition({ x: clientX, y: clientY })
    const id = newId(kind)
    const rect = kind === 'sticky'
      ? { x: at.x - 100, y: at.y - 100, width: 200, height: 200 }
      : { x: at.x, y: at.y - 20, width: 240, height: 48 }
    commit({ nodes: [{ id, type: 'text', text: '', ...(kind === 'sticky' ? { color: '3' } : {}), ...rounded(rect) }, ...sectionFor(rect)] })
    setEditing(id)
    setTool('select')
  }

  const addSection = (rect: Rect) => {
    commit({ nodes: [{ id: newId('section'), type: 'group', label: 'Section', ...rounded(rect) }] })
    setTool('select')
  }

  const onLayerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    if (tool === 'section') setDraft({ x0: event.clientX, y0: event.clientY, x1: event.clientX, y1: event.clientY })
  }

  const onLayerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!draft) {
      create(tool, event.clientX, event.clientY)
      return
    }
    setDraft(null)
    const a = flow.screenToFlowPosition({ x: Math.min(draft.x0, draft.x1), y: Math.min(draft.y0, draft.y1) })
    const b = flow.screenToFlowPosition({ x: Math.max(draft.x0, draft.x1), y: Math.max(draft.y0, draft.y1) })
    const small = Math.abs(draft.x1 - draft.x0) < 12 && Math.abs(draft.y1 - draft.y0) < 12
    addSection(small ? { x: a.x - 240, y: a.y - 160, width: 480, height: 320 } : { x: a.x, y: a.y, width: b.x - a.x, height: b.y - a.y })
  }

  const onConnect = (connection: Connection) => {
    commit({ edges: [{ id: newId('edge'), fromNode: connection.source, toNode: connection.target, fromSide: connection.sourceHandle, toSide: connection.targetHandle }] })
  }

  const onReconnect = (old: BoardEdge, connection: Connection) => {
    commit({ edges: [{ id: old.id, fromNode: connection.source, toNode: connection.target, fromSide: connection.sourceHandle, toSide: connection.targetHandle }] })
  }

  const onNodeClick = (_event: unknown, node: BoardNode) => {
    if (tool !== 'connector') return
    if (!connectSource) {
      setConnectSource(node.id)
      return
    }
    if (node.id === connectSource) return
    const [fromSide, toSide] = sidesBetween(rectOfNode(flow.getNode(connectSource)!), rectOfNode(node))
    commit({ edges: [{ id: newId('edge'), fromNode: connectSource, toNode: node.id, fromSide, toSide }] })
    setConnectSource(null)
    setTool('select')
  }

  const chooseTool = (next: Tool) => {
    setTool(next)
    setConnectSource(null)
  }

  const keys: Bindings = {
    ...Object.fromEntries(Object.entries(TOOL_KEYS).map(([key, next]) => [key, () => chooseTool(next)])),
    Escape: () => {
      setFocused(null)
      chooseTool('select')
    },
    // Shift+S wraps the selection in a section, or draws one.
    'Shift+S': () => {
      const picked = flow.getNodes().filter((node) => node.selected && node.type !== 'group')
      if (!picked.length) {
        chooseTool('section')
        return
      }
      addSection(padded(bounds(picked.map(rectOfNode))))
    },
  }
  useCanvasInput({ local, commit, replay, onSelect, container, keys })

  const board = useMemo<CanvasBoard>(() => ({
    sessionId,
    view,
    theme,
    focused,
    setFocused,
    flash,
    editing,
    setEditing,
    dropTarget,
    connectSource,
    commit,
    prompt: onPrompt,
  }), [sessionId, view, theme, focused, flash, editing, dropTarget, connectSource, commit, onPrompt])

  const focusedItem = nodes.find((node) => node.id === focused)?.data.item
  const focusedKind = focusedItem?.type === 'link' ? 'link' : viewerFor(String(focusedItem?.file ?? ''))

  return (
    <div
      ref={container}
      className={`canvas tool-${tool}${busy || dragging ? ' canvas-busy' : ''}${tool === 'connector' ? ' canvas-connecting' : ''}`}
      style={{ '--canvas-zoom': zoom, '--canvas-inset': `${inset}px` } as CSSProperties}
      onPointerDownCapture={touch}
      onWheelCapture={touch}
      onKeyDownCapture={touch}
    >
      <CanvasContext.Provider value={board}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onReconnect={onReconnect}
          onDelete={({ nodes: gone, edges: cut }) => {
            commit({ remove: [...new Set([...withContents(gone, flow.getNodes()), ...cut].map((item) => item.id))] })
          }}
          {...drag}
          onNodeClick={onNodeClick}
          onNodeDoubleClick={(event, node) => {
            if (node.type !== 'group') {
              if (focusable(node.data.item)) setFocused(node.id)
              return
            }
            // Double-clicking a section's edge fits it to what it holds.
            const inside = sectionContents(node, flow.getNodes())
            if (inside.length && (event.target as HTMLElement).closest('.react-flow__resize-control')) {
              commit({ nodes: [{ id: node.id, ...rounded(padded(bounds(inside.map(rectOfNode)))) }] })
            }
          }}
          onEdgeDoubleClick={(_event, edge) => setEditing(edge.id)}
          onPaneClick={() => setFocused(null)}
          onSelectionChange={onSelectionChange}
          onMoveStart={(event) => event && setBusy(true)}
          onMoveEnd={() => setBusy(false)}
          onSelectionStart={() => setBusy(true)}
          onSelectionEnd={() => setBusy(false)}
          connectionMode={ConnectionMode.Loose}
          connectionLineStyle={{ stroke: 'var(--canvas-accent)', strokeWidth: 1.5 }}
          reconnectRadius={8}
          panOnScroll
          selectionOnDrag={tool !== 'hand'}
          panOnDrag={tool === 'hand' ? true : [1, 2]}
          nodesDraggable={tool !== 'hand'}
          elementsSelectable={tool !== 'hand'}
          zoomOnDoubleClick={false}
          elevateNodesOnSelect={false}
          nodeDragThreshold={3}
          deleteKeyCode={['Backspace', 'Delete']}
          multiSelectionKeyCode={['Meta', 'Shift']}
          minZoom={0.1}
          maxZoom={4}
          proOptions={{ hideAttribution: true }}
        >
          <Background variant={BackgroundVariant.Dots} gap={22} size={1.6} color="var(--canvas-dot)" bgColor="var(--color-bg)" />
          <Panel position="bottom-right" className="canvas-map canvas-overlay">
            <MiniMap
              pannable
              zoomable
              ariaLabel="Canvas overview"
              nodeColor={(node) => (node.type === 'group' ? 'var(--canvas-map-section)' : 'var(--canvas-map-node)')}
              nodeStrokeWidth={0}
              maskColor="var(--canvas-map-mask)"
              style={{ position: 'static', margin: 0, width: 200, height: 130, background: 'transparent' }}
            />
            <div className="canvas-zoom">
              <button type="button" aria-label="Zoom out" onClick={() => void flow.zoomOut({ duration: 200 })}>
                <Minus size={14} />
              </button>
              <button type="button" title="Fit everything" onClick={() => void flow.fitView({ duration: 300, ...fitPadding(inset) })}>
                {Math.round(zoom * 100)}%
              </button>
              <button type="button" aria-label="Zoom in" onClick={() => void flow.zoomIn({ duration: 200 })}>
                <Plus size={14} />
              </button>
            </div>
          </Panel>
        </ReactFlow>
      </CanvasContext.Provider>

      {['sticky', 'text', 'section'].includes(tool) ? (
        <div
          className="canvas-tool-layer"
          onPointerDown={onLayerDown}
          onPointerMove={(event) => draft && setDraft({ ...draft, x1: event.clientX, y1: event.clientY })}
          onPointerUp={onLayerUp}
        >
          {draft ? (
            <div
              className="canvas-section-draft"
              style={{ left: Math.min(draft.x0, draft.x1) - (container.current?.getBoundingClientRect().left ?? 0), top: Math.min(draft.y0, draft.y1) - (container.current?.getBoundingClientRect().top ?? 0), width: Math.abs(draft.x1 - draft.x0), height: Math.abs(draft.y1 - draft.y0) }}
            />
          ) : null}
        </div>
      ) : null}

      <CanvasDock tool={tool} onTool={chooseTool} />

      {focusedItem ? (
        <div className="canvas-overlay canvas-banner">
          <span className="canvas-banner-dot" />
          <span>
            Focused on <strong>{focusedItem.type === 'link' ? String(focusedItem.url) : fileName(String(focusedItem.file))}</strong>
          </span>
          <span className="text-ink-3">{HINTS[focusedKind] ?? ''} · Esc</span>
          <button type="button" onClick={() => setFocused(null)}>Done</button>
        </div>
      ) : news.length ? (
        <div className="canvas-overlay canvas-banner">
          <span className="canvas-banner-dot" />
          <span>Jaz changed {news.length === 1 ? 'an item' : `${news.length} items`}</span>
          <button type="button" onClick={() => show(news)}>Show</button>
        </div>
      ) : error ? (
        <div className="canvas-overlay canvas-banner danger">{error.message}</div>
      ) : null}
    </div>
  )
}
