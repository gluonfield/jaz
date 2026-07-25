import { useRef } from 'react'

// Streamed text arrives as whole chunks appended to one growing string, so the
// only thing that must animate is the delta. The tail is wrapped after parsing
// (never by editing the Markdown source) and the phase alternates so the CSS
// animation restarts on every chunk even when React reuses the span.

// Each wrapped line fragment carries its own copy of the mask, so a tail wider
// than a couple of lines wipes them in parallel instead of in reading order.
// Past the cap the older part of the chunk simply lands settled.
const MAX_TAIL_CHARS = 240

export type StreamTail = { text: string; chars: number; phase: 'a' | 'b' }

// A message that mounts whole (history, or a turn's persisted row replacing the
// live event) has no delta to reveal, so only growth of the same string counts.
export function nextStreamTail(prev: StreamTail, text: string): StreamTail {
  if (prev.text === text) return prev
  const appended = prev.text !== '' && text.length > prev.text.length && text.startsWith(prev.text)
  return {
    text,
    chars: appended ? Math.min(text.length - prev.text.length, MAX_TAIL_CHARS) : 0,
    phase: prev.phase === 'a' ? 'b' : 'a',
  }
}

export function useStreamTail(text: string): StreamTail {
  const ref = useRef<StreamTail>({ text: '', chars: 0, phase: 'a' })
  ref.current = nextStreamTail(ref.current, text)
  return ref.current
}

type HastNode = {
  type: string
  tagName?: string
  value?: string
  properties?: Record<string, unknown>
  children?: HastNode[]
}

// Code and math render from their own text, so a wrapper inside them would
// corrupt the block. Their text still spends the budget: the reveal must stay
// anchored to the end of the message instead of sliding back over settled prose.
function isOpaque(node: HastNode): boolean {
  if (node.tagName === 'pre' || node.tagName === 'code') return true
  const className = node.properties?.className
  return Array.isArray(className) && className.some((name) => String(name).startsWith('math'))
}

export function rehypeStreamTail({ chars, phase }: StreamTail) {
  return (tree: HastNode) => {
    if (chars > 0) wrapTail(tree, { left: chars, phase }, false)
  }
}

function wrapTail(node: HastNode, budget: { left: number; phase: string }, opaque: boolean): void {
  const children = node.children
  if (!children) return
  for (let i = children.length - 1; i >= 0 && budget.left > 0; i--) {
    const child = children[i]
    if (child.type === 'element') {
      wrapTail(child, budget, opaque || isOpaque(child))
      continue
    }
    if (child.type !== 'text') continue
    const value = child.value ?? ''
    const start = Math.max(0, value.length - budget.left)
    budget.left -= value.length - start
    const tail = value.slice(start)
    if (opaque || !tail.trim()) continue
    const span: HastNode = {
      type: 'element',
      tagName: 'span',
      properties: { dataStreamTail: budget.phase },
      children: [{ type: 'text', value: tail }],
    }
    if (start === 0) {
      children[i] = span
    } else {
      child.value = value.slice(0, start)
      children.splice(i + 1, 0, span)
    }
  }
}
