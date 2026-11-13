import { useReactFlow } from '@xyflow/react'
import { useCallback, useEffect, useEffectEvent, useRef, type RefObject } from 'react'
import type { CanvasItem, CanvasPatch, CanvasView } from '@/lib/api/canvas'
import { bounds, centre, cloneItems, itemLabel, rectOfItem, viewRect, withContents, type BoardEdge, type BoardNode, type Clip } from './board'
import { WEB_ADDRESS } from './format'
import type { CanvasPick } from './useCanvasContexts'

const CLIP = 'application/x-jaz-canvas'
const OFFSET = 24
const ARROWS: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }

// A binding returns false when it had nothing to act on, leaving the key to
// the rest of the page.
export type Bindings = Record<string, () => boolean | void>

function pick(page: string, item: CanvasItem): CanvasPick {
  return { page, node: item.id, label: itemLabel(item) }
}

// Letters go by key position, so shortcuts survive other keyboard layouts.
function chord(event: KeyboardEvent): string {
  const key = event.code.startsWith('Key') ? event.code.slice(3) : event.key
  return [(event.metaKey || event.ctrlKey) && 'Mod', event.shiftKey && 'Shift', key].filter(Boolean).join('+')
}

// The board's keys and clipboard: undo and redo, select all, duplicate, nudge,
// copy, cut and paste, plus the caller's own keys. They act while the board
// holds the user's attention: focus inside it, or a last click on it with
// nothing focused.
export function useCanvasInput({ page, local, commit, replay, onSelect, onFiles, container, keys }: {
  page: string
  local: CanvasView
  commit: (change: CanvasPatch) => void
  replay: (from: 'undo' | 'redo') => void
  onSelect: (picks: CanvasPick[]) => void
  onFiles: (files: File[]) => void
  container: RefObject<HTMLDivElement | null>
  keys: Bindings
}) {
  const flow = useReactFlow<BoardNode, BoardEdge>()
  const clicked = useRef(false)
  const pasted = useRef({ raw: '', count: 0 })

  const attending = (event: Event) => {
    const target = event.target as Element
    if (target.closest('input, textarea, [contenteditable="true"]')) {
      return false
    }
    return !!container.current?.contains(target) || (target === document.body && clicked.current)
  }

  // The selection with what its sections hold and the connectors among them.
  const picked = useCallback((): Clip => {
    const all = flow.getNodes()
    const ids = new Set(withContents(all.filter((node) => node.selected), all).map((node) => node.id))
    return {
      nodes: local.nodes.filter((item) => ids.has(item.id)),
      edges: local.edges.filter((edge) => ids.has(edge.fromNode as string) && ids.has(edge.toNode as string)),
    }
  }, [flow, local])

  const place = useCallback((clip: Clip, dx: number, dy: number) => {
    const copy = cloneItems(clip, dx, dy)
    commit(copy)
    onSelect(copy.nodes.map((item) => pick(page, item)))
  }, [commit, onSelect, page])

  const withSelection = (act: (clip: Clip) => void) => () => {
    const clip = picked()
    if (!clip.nodes.length) {
      return false
    }
    act(clip)
  }

  const duplicate = useCallback(() => {
    const clip = picked()
    if (!clip.nodes.length) {
      return false
    }
    place(clip, OFFSET, OFFSET)
  }, [picked, place])

  const nudge = (dx: number, dy: number) => withSelection(({ nodes }) => {
    commit({ nodes: nodes.filter((item) => item.locked !== true).map((item) => ({ id: item.id, x: rectOfItem(item).x + dx, y: rectOfItem(item).y + dy })) })
  })

  // Cmd+Delete removes sections and keeps what they hold; Delete takes it all.
  const dissolve = () => {
    const sections = flow.getNodes().filter((node) => node.selected && node.type === 'group')
    if (!sections.length) {
      return false
    }
    commit({ remove: sections.map((node) => node.id) })
  }

  const bindings = (): Bindings => ({
    'Mod+Z': () => replay('undo'),
    'Mod+Shift+Z': () => replay('redo'),
    'Mod+Y': () => replay('redo'),
    'Mod+A': () => onSelect(local.nodes.map((item) => pick(page, item))),
    'Mod+D': duplicate,
    'Mod+Backspace': dissolve,
    'Mod+Delete': dissolve,
    ...Object.fromEntries(Object.entries(ARROWS).flatMap(([key, [dx, dy]]) => [
      [key, nudge(dx, dy)],
      [`Shift+${key}`, nudge(dx * 10, dy * 10)],
    ])),
    ...keys,
  })

  const onKey = useEffectEvent((event: KeyboardEvent) => {
    const action = bindings()[chord(event)]
    if (!action || event.altKey || event.defaultPrevented || !attending(event)) {
      return
    }
    if (action() !== false) {
      event.preventDefault()
      event.stopPropagation()
    }
  })

  const onCopy = useEffectEvent((event: ClipboardEvent) => {
    if (document.getSelection()?.isCollapsed === false) {
      return
    }
    const clip = attending(event) ? picked() : undefined
    if (!clip?.nodes.length) {
      return
    }
    event.preventDefault()
    event.clipboardData?.setData(CLIP, JSON.stringify({ ...clip, dir: local.dir }))
    event.clipboardData?.setData('text/plain', clip.nodes.map(itemLabel).join('\n'))
    if (event.type === 'cut') {
      commit({ remove: clip.nodes.map((item) => item.id) })
    }
  })

  // Words from elsewhere become a text item in the middle of the view, and a
  // web address a page.
  const pasteWords = (words: string) => {
    const middle = centre(viewRect(flow, container.current!))
    const item = WEB_ADDRESS.test(words) ? { type: 'link', url: words, width: 480, height: 320 } : { type: 'text', text: words, width: 320 }
    place({ nodes: [{ id: '', ...item, x: middle.x - item.width / 2, y: middle.y - 80 }], edges: [] }, 0, 0)
  }

  // Copied items land beside their original while that is in view, otherwise
  // in the middle of the view; pasting again steps further along. Files from
  // another board keep pointing at its folder.
  const pasteClip = (raw: string) => {
    const { dir, nodes, edges } = JSON.parse(raw) as Clip & { dir: string }
    const rebased = nodes.map((item) => (dir !== local.dir && typeof item.file === 'string' && !item.file.startsWith('/') ? { ...item, file: `${dir}/${item.file}` } : item))
    const count = pasted.current.raw === raw ? pasted.current.count + 1 : 1
    pasted.current = { raw, count }
    const box = bounds(rebased.map(rectOfItem))
    const view = viewRect(flow, container.current!)
    const seen = box.x < view.x + view.width && box.x + box.width > view.x && box.y < view.y + view.height && box.y + box.height > view.y
    const dx = seen ? 0 : centre(view).x - centre(box).x
    const dy = seen ? 0 : centre(view).y - centre(box).y
    place({ nodes: rebased, edges }, dx + OFFSET * count, dy + OFFSET * count)
  }

  // Pictures and files pasted from elsewhere are the caller's to add.
  const onPaste = useEffectEvent((event: ClipboardEvent) => {
    const raw = event.clipboardData?.getData(CLIP)
    const files = [...(event.clipboardData?.files ?? [])]
    const words = event.clipboardData?.getData('text/plain').trim()
    if ((!raw && !files.length && !words) || !attending(event)) {
      return
    }
    event.preventDefault()
    if (raw) pasteClip(raw)
    else if (files.length) onFiles(files)
    else pasteWords(words!)
  })

  useEffect(() => {
    const key = (event: KeyboardEvent) => onKey(event)
    const copy = (event: ClipboardEvent) => onCopy(event)
    const paste = (event: ClipboardEvent) => onPaste(event)
    const down = (event: PointerEvent) => {
      clicked.current = !!container.current?.contains(event.target as Node)
    }
    window.addEventListener('keydown', key, true)
    window.addEventListener('pointerdown', down, true)
    document.addEventListener('copy', copy)
    document.addEventListener('cut', copy)
    document.addEventListener('paste', paste)
    return () => {
      window.removeEventListener('keydown', key, true)
      window.removeEventListener('pointerdown', down, true)
      document.removeEventListener('copy', copy)
      document.removeEventListener('cut', copy)
      document.removeEventListener('paste', paste)
    }
  }, [container])
  return duplicate
}
