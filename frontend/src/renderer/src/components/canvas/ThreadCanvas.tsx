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
  type Viewport,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import './canvas.css'
import './canvas-format.css'
import { Minus, Plus } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import type { CanvasPatch, CanvasView } from '@/lib/api/canvas'
import { useToast } from '@/components/ui/toast'
import { apiBaseUrl } from '@/lib/api/client'
import { useArtifactThemeCSS } from '@/lib/hooks/useArtifactThemeCSS'
import { layerItems } from './arrange'
import {
  bounds,
  centre,
  fileName,
  focusable,
  itemLabel,
  mergeEdges,
  mergeNodes,
  newId,
  rectOfNode,
  padded,
  rounded,
  sectionAround,
  sectionContents,
  sidesBetween,
  toFlowEdges,
  toFlowNodes,
  viewerFor,
  viewRect,
  withContents,
  type BoardEdge,
  type BoardNode,
} from './board'
import { CanvasContext, type CanvasBoard } from './canvasContext'
import { CanvasDock, type Tool } from './CanvasDock'
import { ConnectorEdge } from './CanvasEdges'
import { FileNode, GroupNode, LinkNode, TextNode } from './CanvasNodes'
import { StrokeNode } from './CanvasStroke'
import { TableNode } from './CanvasTable'
import { TimerNode } from './CanvasTimer'
import { CommentBadges } from './CanvasComments'
import { SelectionToolbar } from './CanvasToolbar'
import { ToolLayer } from './CanvasTools'
import { fitPadding, useAgentFollow } from './useAgentFollow'
import { CanvasPages, type PageTabsMemory } from '@/components/canvas/CanvasPages'
import type { CanvasPick } from './useCanvasContexts'
import { uploadedItems } from './upload'
import { useCanvasLayout } from './useCanvasLayout'
import { useCanvasInput, type Bindings } from './useCanvasInput'
import { useCanvasSync } from './useCanvasSync'
import { useSectionDrag } from './useSectionDrag'
import { readCanvasLocation, saveCanvasLocation } from '@/components/canvas/canvasLocation'

interface Props {
  sessionId: string
  // Width of the board's left edge that the chat covers.
  inset: number
  working: boolean
  bindSelection: (select: (picks: CanvasPick[]) => void) => () => void
  page?: string
  item?: string
  navigationKey?: string
  onPage: (page: { id: string; name: string }) => void
  onSelect: (picks: CanvasPick[]) => void
  onPrompt: (text: string) => void
}

const nodeTypes = { text: TextNode, file: FileNode, link: LinkNode, group: GroupNode, table: TableNode, stroke: StrokeNode, timer: TimerNode }
const edgeTypes = { connector: ConnectorEdge }
const TOOL_KEYS: Record<string, Tool> = { V: 'select', H: 'hand', S: 'sticky', T: 'text', R: 'shape', B: 'table', X: 'connector', P: 'pen', E: 'eraser', K: 'sticker', M: 'timer' }
const HINTS: Record<string, string> = { link: 'Use the page', cad: 'Drag to rotate, scroll to zoom', dxf: 'Drag to pan, scroll to zoom', pdf: 'Scroll and use the page controls' }

export function ThreadCanvas(props: Props) {
  const storageKey = `jaz.canvasPage.${apiBaseUrl()}/${props.sessionId}`
  return <SessionCanvas key={storageKey} {...props} storageKey={storageKey} />
}

function SessionCanvas({ storageKey, ...props }: Props & { storageKey: string }) {
  const { onPage } = props
  const sync = useCanvasSync(props.sessionId, props.working)
  const [location, setLocation] = useState(() => readCanvasLocation(storageKey))
  const [jump, setJump] = useState<{ page: string; item: string } | null>(null)
  const [navigationError, setNavigationError] = useState('')
  const visited = useRef(location.navigation)
  const viewports = useRef(new Map<string, Viewport>())
  const tabs = useRef<PageTabsMemory>({ scroll: 0, focus: false })
  const selectedPage = sync.local?.pages.find((page) => page.id === location.page)
  const page = selectedPage ?? sync.local?.pages[0]
  useEffect(() => {
    if (page) {
      if (selectedPage || sync.refreshed) {
        setLocation((current) => current.page === page.id ? current : { ...current, page: page.id })
      }
      onPage({ id: page.id, name: page.name })
    }
  }, [page, selectedPage, sync.refreshed, onPage])
  useEffect(() => {
    if (location.page) saveCanvasLocation(storageKey, location)
  }, [storageKey, location])
  const choosePage = (id: string) => {
    setLocation((current) => ({ ...current, page: id }))
    setJump(null)
    setNavigationError('')
  }
  useEffect(() => {
    if (!sync.local || (!props.page && !props.item)) return
    const key = JSON.stringify([props.navigationKey, props.page, props.item])
    if (visited.current === key) return
    const target = sync.local.pages.find((page) => page.id === props.page)
    const missingItem = Boolean(target && props.item && !target.nodes.some((item) => item.id === props.item))
    if ((!target || missingItem) && !sync.refreshed) return
    visited.current = key
    setJump(null)
    if (!target) {
      setNavigationError(props.page ? `Page “${props.page}” was not found.` : 'An item link must specify its page.')
      return
    }
    if (missingItem) {
      setNavigationError(`Item “${props.item}” was not found on ${target.name}.`)
      return
    }
    setLocation({ page: target.id, navigation: key })
    setJump(props.item ? { page: target.id, item: props.item } : null)
    setNavigationError('')
  }, [props.navigationKey, props.page, props.item, sync.local, sync.refreshed])
  if (!sync.view || !sync.local || !page) {
    return <div className="canvas">{sync.error ? <div className="canvas-overlay canvas-banner danger">{sync.error.message}</div> : null}</div>
  }
  const local = { ...page, version: sync.local.version, dir: sync.local.dir }
  const view = { ...(sync.view.pages.find((entry) => entry.id === page.id) ?? page), version: sync.view.version, dir: sync.view.dir }
  return (
    <ReactFlowProvider key={page.id}>
      <Board
        {...props}
        pageId={page.id}
        view={view}
        local={local}
        commit={(change) => sync.commit(page.id, change)}
        apply={(change) => sync.apply(page.id, change)}
        replay={(from) => sync.replay(page.id, from)}
        error={sync.error}
        initialViewport={viewports.current.get(page.id)}
        onViewport={(viewport) => { viewports.current.set(page.id, viewport) }}
        targetItem={jump?.page === page.id ? jump.item : undefined}
        onJump={() => setJump((current) => current?.page === page.id && current.item === jump?.item ? null : current)}
      >
        <CanvasPages pages={sync.local.pages} page={page} memory={tabs} onPage={choosePage} onCreate={async () => {
          const id = newId('page')
          await sync.createPage(id)
          choosePage(id)
        }} onRename={sync.renamePage} onDelete={sync.deletePage} />
        {navigationError ? <div className="canvas-overlay canvas-banner danger" role="alert">{navigationError}</div> : null}
      </Board>
    </ReactFlowProvider>
  )
}

interface BoardProps extends Props {
  pageId: string
  initialViewport?: Viewport
  onViewport: (viewport: Viewport) => void
  targetItem?: string
  onJump: () => void
  children: ReactNode
  view: CanvasView
  local: CanvasView
  commit: (change: CanvasPatch) => void
  apply: (change: CanvasPatch) => void
  replay: (from: 'undo' | 'redo') => void
  error: Error | null
}

function Board({ sessionId, pageId, inset, bindSelection, onSelect, onPrompt, view, local, commit, apply, replay, error, initialViewport, onViewport, targetItem, onJump, children }: BoardProps) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const { zoom } = useViewport()
  const theme = useArtifactThemeCSS()
  const container = useRef<HTMLDivElement>(null)
  const picker = useRef<HTMLInputElement>(null)
  const toast = useToast()
  const { flash, news, show, touch, settle } = useAgentFollow(view, local, container, inset, !initialViewport && !targetItem)
  const [nodes, setNodes, onNodesChange] = useNodesState<BoardNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<BoardEdge>([])
  const { dragging, dropTarget, ...drag } = useSectionDrag(setNodes, commit)
  // Nothing from the server lands mid-drag; the drag's own result merges after.
  // Items the user just made arrive selected, so their controls are at hand.
  const made = useRef<string[]>([])
  useEffect(() => {
    if (!dragging) {
      const fresh = made.current
      made.current = []
      setNodes((current) => {
        const arriving = fresh.filter((id) => !current.some((node) => node.id === id))
        return mergeNodes(current, toFlowNodes(local), new Set(arriving.length ? arriving : current.filter((node) => node.selected).map((node) => node.id)))
      })
    }
  }, [local, dragging, setNodes])
  useEffect(() => {
    if (!dragging) {
      setEdges((current) => mergeEdges(current, toFlowEdges(local)))
    }
  }, [local, dragging, setEdges])

  const select = useCallback((picks: CanvasPick[]) => {
    made.current = picks.map((pick) => pick.node)
    setNodes((current) => current.map((node) => ({ ...node, selected: picks.some((pick) => pick.page === pageId && pick.node === node.id) })))
  }, [pageId, setNodes])
  useEffect(() => bindSelection(select), [bindSelection, select])

  const [tool, setTool] = useState<Tool>('select')
  const [editing, setEditing] = useState<string | null>(null)
  const [focused, setFocused] = useState<string | null>(null)
  const [connectSource, setConnectSource] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useCanvasLayout({ sessionId, page: pageId, local, nodes, hold: dragging || editing !== null, apply, container })

  const onSelectionChange = useCallback(({ nodes: picked }: OnSelectionChangeParams<BoardNode, BoardEdge>) => {
    onSelect(picked.map((node) => ({ page: pageId, node: node.id, label: itemLabel(node.data.item) })))
  }, [onSelect, pageId])

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

  // Pictures and files go into the canvas folder and onto the board, at the
  // point given or the middle of the view.
  const addFiles = async (files: File[], at?: { x: number; y: number }) => {
    const middle = at ? flow.screenToFlowPosition(at) : centre(viewRect(flow, container.current!))
    try {
      const items = await uploadedItems(sessionId, files, middle)
      made.current = items.map((item) => item.id!)
      commit({ nodes: items })
    } catch (error) {
      toast(`Couldn't add the file: ${(error as Error).message}`, 'danger')
    }
  }

  const chooseTool = (next: Tool) => {
    setTool(next)
    setConnectSource(null)
  }

  const restack = (to: 'front' | 'back') => () => {
    const ids = new Set(flow.getNodes().filter((node) => node.selected).map((node) => node.id))
    const change = layerItems(local.nodes, ids, to)
    if (!change.length) return false
    commit({ nodes: change })
  }

  const keys: Bindings = {
    ...Object.fromEntries(Object.entries(TOOL_KEYS).map(([key, next]) => [key, () => chooseTool(next)])),
    I: () => picker.current?.click(),
    'Mod+]': restack('front'),
    'Mod+[': restack('back'),
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
      commit({ nodes: [sectionAround(picked)] })
      setTool('select')
    },
  }
  const duplicate = useCanvasInput({ page: pageId, local, commit, replay, onSelect: select, onFiles: (files) => void addFiles(files), container, keys })

  useEffect(() => {
    if (!targetItem || !nodes.some((node) => node.id === targetItem)) return
    setNodes((current) => current.map((node) => ({ ...node, selected: node.id === targetItem })))
    requestAnimationFrame(() => void flow.fitView({ nodes: [{ id: targetItem }], duration: 300, maxZoom: 1, ...fitPadding(inset) }))
    onJump()
  }, [targetItem, nodes, setNodes, flow, inset, onJump])

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
    duplicate,
    prompt: onPrompt,
  }), [sessionId, view, theme, focused, flash, editing, dropTarget, connectSource, commit, duplicate, onPrompt])

  const picked = nodes.filter((node) => node.selected)
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
      onDragOver={(event) => event.dataTransfer.types.includes('Files') && event.preventDefault()}
      onDrop={(event) => {
        const files = [...event.dataTransfer.files]
        if (!files.length) return
        event.preventDefault()
        void addFiles(files, { x: event.clientX, y: event.clientY })
      }}
    >
      <input
        ref={picker}
        type="file"
        multiple
        hidden
        aria-label="Add pictures or files"
        onChange={(event) => {
          void addFiles([...(event.target.files ?? [])])
          event.target.value = ''
        }}
      />
      <CanvasContext.Provider value={board}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={(changes) => {
            onNodesChange(changes)
            if (changes.some((change) => change.type === 'dimensions')) settle()
          }}
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
          onMoveEnd={(_event, viewport) => {
            setBusy(false)
            onViewport(viewport)
          }}
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
          defaultViewport={initialViewport}
          minZoom={0.1}
          maxZoom={4}
          proOptions={{ hideAttribution: true }}
        >
          {picked.length > 1 ? <SelectionToolbar nodes={picked} /> : null}
          <CommentBadges nodes={nodes} />
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
        <ToolLayer tool={tool} onMade={(id) => { made.current = [id] }} onDone={() => setTool('select')} />
      </CanvasContext.Provider>

      {children}
      <CanvasDock tool={tool} onTool={chooseTool} onUpload={() => picker.current?.click()} />

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
