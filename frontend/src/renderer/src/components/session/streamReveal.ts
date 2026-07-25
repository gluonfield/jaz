import type { Element, Root, RootContent } from 'hast'
import { useRef } from 'react'

// Streamed text is one growing string, so the only thing that should animate is
// what the latest chunk added. Parsed nodes keep their source offsets, so the
// reveal is anchored to where the previous render ended rather than counting
// characters back from the end — Markdown syntax and rendered text are
// different lengths, and counting backwards drags the sweep over settled prose.
// The phase alternates because react-markdown keys children by `tagName-count`:
// the tail span keeps the same key every chunk, so React reuses the node and
// only a changed animation-name restarts the sweep.

// Each wrapped line fragment carries its own copy of the mask, so a tail wider
// than a couple of lines wipes them in parallel instead of in reading order.
// Past the cap the older part of the chunk simply lands settled.
const MAX_TAIL_CHARS = 240

export type StreamTail = { text: string; offset: number; phase: 'a' | 'b' }

// A message that mounts whole (history, or a turn's persisted row replacing the
// live event) has no delta to reveal, so only growth of the same string counts;
// an offset at the end of the text reveals nothing.
export function nextStreamTail(prev: StreamTail, text: string): StreamTail {
  if (prev.text === text) return prev
  const appended = prev.text !== '' && text.length > prev.text.length && text.startsWith(prev.text)
  return {
    text,
    offset: appended ? Math.max(prev.text.length, text.length - MAX_TAIL_CHARS) : text.length,
    phase: prev.phase === 'a' ? 'b' : 'a',
  }
}

// Keyed on the text it already saw, so a repeated render — StrictMode's double
// invoke included — resolves to the same tail instead of consuming the growth.
export function useStreamTail(text: string): StreamTail {
  const ref = useRef<StreamTail>({ text: '', offset: 0, phase: 'a' })
  ref.current = nextStreamTail(ref.current, text)
  return ref.current
}

export function rehypeStreamTail({ text, offset, phase }: StreamTail) {
  return (tree: Root) => {
    if (offset < text.length) revealTail(tree.children, offset, phase)
  }
}

// Code and math render from their own text, so a wrapper inside them would
// corrupt the block; their content lands settled instead.
function isOpaque(node: Element): boolean {
  if (node.tagName === 'pre' || node.tagName === 'code') return true
  const className = node.properties.className
  return Array.isArray(className) && className.some((name) => String(name).startsWith('math'))
}

// Nodes a transform synthesized (linkified file paths) carry no source offset
// and stay settled rather than guessing where they came from.
function revealTail(children: RootContent[], offset: number, phase: string): void {
  for (let i = 0; i < children.length; i++) {
    const child = children[i]
    if (child.type === 'element') {
      if (!isOpaque(child)) revealTail(child.children, offset, phase)
      continue
    }
    if (child.type !== 'text') continue
    const start = child.position?.start.offset
    if (start === undefined) continue
    const settled = Math.max(0, offset - start)
    const tail = child.value.slice(settled)
    if (!tail.trim()) continue
    const span: Element = {
      type: 'element',
      tagName: 'span',
      properties: { dataStreamTail: phase },
      children: [{ type: 'text', value: tail }],
    }
    if (settled === 0) {
      children[i] = span
    } else {
      child.value = child.value.slice(0, settled)
      children.splice(i + 1, 0, span)
      i++
    }
  }
}
