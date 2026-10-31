import { MarkerType, type Edge, type Node } from '@xyflow/react'
import type { CanvasBoard, CanvasDocument, CanvasItem, CanvasPagePatch, CanvasPatch, CanvasView } from '@/lib/api/canvas'

export type Side = 'top' | 'right' | 'bottom' | 'left'
export type Rect = { x: number; y: number; width: number; height: number }
export type Kind = 'text' | 'file' | 'link' | 'group'

export type BoardNode = Node<{ item: CanvasItem }, Kind>
export type BoardEdge = Edge<{ label?: string }, 'connector'>
export type Clip = { nodes: CanvasItem[]; edges: CanvasItem[] }

export const SECTION_PAD = 36
const KINDS: Kind[] = ['text', 'file', 'link', 'group']
const SIDES: Side[] = ['top', 'right', 'bottom', 'left']

export const arrow = { type: MarkerType.ArrowClosed, width: 16, height: 16, color: 'var(--canvas-edge)' }

function number(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

export function rectOfItem(item: CanvasItem): Rect {
  return { x: number(item.x), y: number(item.y), width: number(item.width), height: number(item.height) }
}

export function rectOfNode(node: BoardNode): Rect {
  return {
    x: node.position.x,
    y: node.position.y,
    width: node.width ?? node.measured?.width ?? 0,
    height: node.height ?? node.measured?.height ?? 0,
  }
}

// Sections sit behind everything: React Flow paints in array order.
export function toFlowNodes(view: CanvasView): BoardNode[] {
  const nodes = view.nodes.flatMap<BoardNode>((item) => {
    const kind = item.type as Kind
    if (!KINDS.includes(kind)) return []
    const rect = rectOfItem(item)
    return [{ id: item.id, type: kind, position: { x: rect.x, y: rect.y }, width: rect.width, height: rect.height, data: { item } }]
  })
  return [...nodes.filter((node) => node.type === 'group'), ...nodes.filter((node) => node.type !== 'group')]
}

export function toFlowEdges(view: CanvasView): BoardEdge[] {
  const rects = new Map(view.nodes.map((item) => [item.id, rectOfItem(item)]))
  return view.edges.flatMap<BoardEdge>((item) => {
    const source = item.fromNode
    const target = item.toNode
    if (typeof source !== 'string' || typeof target !== 'string' || !rects.has(source) || !rects.has(target)) return []
    const [fromSide, toSide] = sidesBetween(rects.get(source)!, rects.get(target)!)
    const label = typeof item.label === 'string' && item.label ? item.label : undefined
    return [{
      id: item.id,
      type: 'connector',
      source,
      target,
      sourceHandle: side(item.fromSide) ?? fromSide,
      targetHandle: side(item.toSide) ?? toSide,
      markerEnd: item.toEnd === 'none' ? undefined : arrow,
      markerStart: item.fromEnd === 'arrow' ? arrow : undefined,
      data: { label },
    }]
  })
}

function side(value: unknown): Side | undefined {
  return SIDES.includes(value as Side) ? (value as Side) : undefined
}

export function sidesBetween(a: Rect, b: Rect): [Side, Side] {
  const dx = b.x + b.width / 2 - (a.x + a.width / 2)
  const dy = b.y + b.height / 2 - (a.y + a.height / 2)
  if (Math.abs(dx) * Math.max(1, a.height + b.height) >= Math.abs(dy) * Math.max(1, a.width + b.width)) {
    return dx >= 0 ? ['right', 'left'] : ['left', 'right']
  }
  return dy >= 0 ? ['bottom', 'top'] : ['top', 'bottom']
}

// The client's optimistic copy of a patch, with the server's semantics: fields
// set to null are removed, unknown ids are added, and removing a node drops
// the edges that touch it.
export function applyPatch<T extends CanvasBoard>(view: T, change: CanvasPatch): T {
  const removed = new Set(change.remove)
  const merge = (items: CanvasItem[], changes: Partial<CanvasItem>[] = []) => {
    const out = items.filter((item) => !removed.has(item.id))
    for (const fields of changes) {
      const at = out.findIndex((item) => item.id === fields.id)
      const item: CanvasItem = at < 0 ? { id: fields.id! } : { ...out[at] }
      for (const [key, value] of Object.entries(fields)) {
        if (value === null) delete item[key]
        else item[key] = value
      }
      if (at < 0) out.push(item)
      else out[at] = item
    }
    return out
  }
  const nodes = merge(view.nodes, change.nodes)
  const ids = new Set(nodes.map((node) => node.id))
  const edges = merge(view.edges, change.edges).filter((edge) => ids.has(edge.fromNode as string) && ids.has(edge.toNode as string))
  return { ...view, nodes, edges }
}

export function applyPagePatch(document: CanvasDocument, change: CanvasPagePatch): CanvasDocument {
  return { ...document, pages: document.pages.map((page) => page.id === change.page ? { ...applyPatch(page, change), name: change.name ?? page.name } : page) }
}

// The patch that undoes `change` on `view`: changed fields get their old
// values back, removed items return with their connectors, added ones go.
export function invertPatch(view: CanvasBoard, change: CanvasPatch): CanvasPatch {
  const after = applyPatch(view, change)
  const restore = (before: CanvasItem[], now: CanvasItem[], changes: Partial<CanvasItem>[] = []) => {
    const present = new Set(now.map((item) => item.id))
    const changed = new Map(changes.map((fields) => [fields.id, fields]))
    const back = before.flatMap<Partial<CanvasItem>>((item) => {
      if (!present.has(item.id)) {
        return [item]
      }
      const fields = changed.get(item.id)
      return fields ? [Object.fromEntries(Object.keys(fields).map((key) => [key, item[key] ?? null]))] : []
    })
    const existed = new Set(before.map((item) => item.id))
    return { back, added: now.filter((item) => !existed.has(item.id)).map((item) => item.id) }
  }
  const nodes = restore(view.nodes, after.nodes, change.nodes)
  const edges = restore(view.edges, after.edges, change.edges)
  return { nodes: nodes.back, edges: edges.back, remove: [...nodes.added, ...edges.added] }
}

export function newId(kind: string): string {
  return `${kind}-${crypto.randomUUID().slice(0, 8)}`
}

// The items again under fresh ids, moved by (dx, dy), connectors following.
export function cloneItems({ nodes, edges }: Clip, dx: number, dy: number): Clip {
  const ids = new Map(nodes.map((item) => [item.id, newId(String(item.type))]))
  return {
    nodes: nodes.map((item) => ({ ...item, id: ids.get(item.id)!, x: Math.round(rectOfItem(item).x + dx), y: Math.round(rectOfItem(item).y + dy) })),
    edges: edges.map((edge) => ({ ...edge, id: newId('edge'), fromNode: ids.get(edge.fromNode as string), toNode: ids.get(edge.toNode as string) })),
  }
}

// The nodes are the server's items, selected as the composer says. An item
// being resized keeps its live geometry, and an unchanged node keeps its
// object so React Flow skips it.
export function mergeNodes(current: BoardNode[], next: BoardNode[], selected: ReadonlySet<string>): BoardNode[] {
  const local = new Map(current.map((node) => [node.id, node]))
  return next.map((node) => {
    const mine = local.get(node.id)
    const chosen = selected.has(node.id)
    if (mine?.resizing || (mine && mine.data.item === node.data.item && Boolean(mine.selected) === chosen)) {
      return mine
    }
    return { ...node, selected: chosen, measured: mine?.measured }
  })
}

export function mergeEdges(current: BoardEdge[], next: BoardEdge[]): BoardEdge[] {
  const selected = new Set(current.filter((edge) => edge.selected).map((edge) => edge.id))
  return next.map((edge) => (selected.has(edge.id) ? { ...edge, selected: true } : edge))
}

// Ids of nodes the new view adds or changes, including files whose content
// changed behind an unchanged node.
export function changedNodeIds(previous: CanvasView, next: CanvasView): string[] {
  const before = new Map(previous.nodes.map((item) => [item.id, item]))
  return next.nodes
    .filter((item) => !sameItem(before.get(item.id), item) || previous.files[item.id]?.version !== next.files[item.id]?.version)
    .map((item) => item.id)
}

// The server writes fields in its own order, so compare field by field.
function sameItem(a: CanvasItem | undefined, b: CanvasItem): boolean {
  const keys = Object.keys(b)
  return !!a && Object.keys(a).length === keys.length && keys.every((key) => JSON.stringify(a[key]) === JSON.stringify(b[key]))
}

// The smallest section that contains the point.
export function innermostSection<T extends { rect: Rect }>(sections: T[], x: number, y: number): T | undefined {
  return sections
    .filter(({ rect }) => containsPoint(rect, x, y))
    .sort((a, b) => a.rect.width * a.rect.height - b.rect.width * b.rect.height)[0]
}

export function contains(outer: Rect, inner: Rect): boolean {
  return inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height
}

function containsPoint(rect: Rect, x: number, y: number): boolean {
  return x >= rect.x && x <= rect.x + rect.width && y >= rect.y && y <= rect.y + rect.height
}

// A section holds the items whose centre lies inside it; it moves, fits and
// deletes with them.
export function sectionContents(section: BoardNode, nodes: BoardNode[]): BoardNode[] {
  const area = rectOfNode(section)
  return nodes.filter((node) => {
    const rect = rectOfNode(node)
    return node.id !== section.id && containsPoint(area, rect.x + rect.width / 2, rect.y + rect.height / 2) && !contains(rect, area)
  })
}

// The nodes with everything their sections hold.
export function withContents(nodes: BoardNode[], all: BoardNode[]): BoardNode[] {
  const sections = nodes.filter((node) => node.type === 'group')
  return [...new Map([...nodes, ...sections.flatMap((section) => sectionContents(section, all))].map((node) => [node.id, node])).values()]
}

export function padded(rect: Rect): Rect {
  return { x: rect.x - SECTION_PAD, y: rect.y - SECTION_PAD, width: rect.width + 2 * SECTION_PAD, height: rect.height + 2 * SECTION_PAD }
}

export function growToFit(section: Rect, items: Rect[]): Rect {
  let { x, y } = section
  let right = section.x + section.width
  let bottom = section.y + section.height
  for (const item of items) {
    x = Math.min(x, item.x - SECTION_PAD)
    y = Math.min(y, item.y - SECTION_PAD)
    right = Math.max(right, item.x + item.width + SECTION_PAD)
    bottom = Math.max(bottom, item.y + item.height + SECTION_PAD)
  }
  return { x, y, width: right - x, height: bottom - y }
}

export function bounds(rects: Rect[]): Rect {
  const x = Math.min(...rects.map((rect) => rect.x))
  const y = Math.min(...rects.map((rect) => rect.y))
  return {
    x,
    y,
    width: Math.max(...rects.map((rect) => rect.x + rect.width)) - x,
    height: Math.max(...rects.map((rect) => rect.y + rect.height)) - y,
  }
}

export function rounded(rect: Rect): Rect {
  return { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) }
}

export function fileName(path: string): string {
  return path.split(/[\\/]/).pop() || path
}

export function extension(path: string): string {
  const name = fileName(path)
  return name.includes('.') ? name.split('.').pop()!.toLowerCase() : ''
}

// The label a selected item carries into the composer and the agent's prompt.
export function itemLabel(item: CanvasItem): string {
  if (typeof item.label === 'string' && item.label) return item.label
  if (item.type === 'group') return 'Section'
  if (item.type === 'file' && typeof item.file === 'string') return fileName(item.file)
  if (item.type === 'link' && typeof item.url === 'string') return item.url
  const text = typeof item.text === 'string' ? item.text.trim().split('\n')[0].replace(/^#+\s*/, '') : ''
  return text.length > 48 ? `${text.slice(0, 47)}…` : text || 'Text'
}

export type Viewer = 'component' | 'image' | 'pdf' | 'cad' | 'dxf' | 'markdown' | 'csv' | 'html' | 'file'

const VIEWERS: Record<string, Viewer> = {
  tsx: 'component',
  jsx: 'component',
  png: 'image',
  jpg: 'image',
  jpeg: 'image',
  gif: 'image',
  webp: 'image',
  svg: 'image',
  avif: 'image',
  pdf: 'pdf',
  step: 'cad',
  stp: 'cad',
  iges: 'cad',
  igs: 'cad',
  brep: 'cad',
  brp: 'cad',
  stl: 'cad',
  obj: 'cad',
  gltf: 'cad',
  glb: 'cad',
  '3mf': 'cad',
  ply: 'cad',
  dxf: 'dxf',
  md: 'markdown',
  markdown: 'markdown',
  txt: 'markdown',
  csv: 'csv',
  html: 'html',
  htm: 'html',
}

export function viewerFor(path: string): Viewer {
  return VIEWERS[extension(path)] ?? 'file'
}

// Viewers that take the pointer for their own gestures; the user focuses them
// first so dragging still moves the item.
export function focusable(item: CanvasItem): boolean {
  return item.type === 'link' || (typeof item.file === 'string' && ['pdf', 'cad', 'dxf'].includes(viewerFor(item.file)))
}

