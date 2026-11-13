import { useEffect, useImperativeHandle, useLayoutEffect, useRef, useState, type Ref, type RefObject } from 'react'
import { WEB_ADDRESS, toggleList, toggleMarkAt } from './format'
import { readRich, richHTML, writeRich, type Block, type Mark, type TextFormat } from './richtext'

export interface TextEditorHandle {
  format: (kind: TextFormat) => void
}

const SOURCE_MARKS: Record<Mark, string> = { bold: '**', italic: '*', strike: '~~', code: '`' }
const COMMANDS: Record<Exclude<TextFormat, 'code' | 'checklist'>, string> = { bold: 'bold', italic: 'italic', strike: 'strikeThrough', list: 'insertUnorderedList', numbered: 'insertOrderedList' }

// React Flow mounts a new node hidden until it is measured, so focus waits
// for it to become visible.
function useFocusWhenShown(field: RefObject<HTMLElement | null>, then: (element: HTMLElement) => void = () => {}) {
  useEffect(() => {
    let frame = 0
    let tries = 0
    const focus = () => {
      field.current?.focus()
      if (document.activeElement === field.current) then(field.current!)
      else if (tries++ < 30) frame = requestAnimationFrame(focus)
    }
    frame = requestAnimationFrame(focus)
    return () => cancelAnimationFrame(frame)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}

function toggleCode() {
  const selection = getSelection()!
  if (!selection.rangeCount) return
  const range = selection.getRangeAt(0)
  const around = range.commonAncestorContainer
  const inside = (around instanceof Element ? around : around.parentElement)?.closest('code')
  if (inside) {
    inside.replaceWith(...inside.childNodes)
    return
  }
  if (range.collapsed) return
  const code = document.createElement('code')
  code.append(range.extractContents())
  range.insertNode(code)
  selection.selectAllChildren(code)
}

// The bullets in the selection become tasks, or plain bullets when all are.
function toggleTasks(root: HTMLElement) {
  const selection = getSelection()!
  const picked = () => [...root.querySelectorAll('li')].filter((item) => selection.containsNode(item, true))
  if (!picked().length || picked().some((item) => item.parentElement!.tagName === 'OL')) document.execCommand('insertUnorderedList')
  const items = picked()
  const all = items.every((item) => item.dataset.checked !== undefined)
  for (const item of items) {
    if (all) delete item.dataset.checked
    else item.dataset.checked ??= 'false'
  }
}

// Words typed as they will look. The element is the browser's to edit; it is
// read back into Markdown when the user leaves it.
export function RichEditor({ ref, blocks, onDone }: { ref: Ref<TextEditorHandle>; blocks: Block[]; onDone: (value: string) => void }) {
  const field = useRef<HTMLDivElement>(null)
  const [start] = useState(() => richHTML(blocks))
  useImperativeHandle(ref, () => ({
    format(kind) {
      if (kind === 'code') toggleCode()
      else if (kind === 'checklist') toggleTasks(field.current!)
      else document.execCommand(COMMANDS[kind])
    },
  }), [])
  useLayoutEffect(() => {
    field.current!.innerHTML = start
    document.execCommand('defaultParagraphSeparator', false, 'p')
  }, [start])
  useFocusWhenShown(field, (element) => {
    getSelection()!.selectAllChildren(element)
    getSelection()!.collapseToEnd()
  })
  return (
    <div
      ref={field}
      className="canvas-text-editor canvas-rich-editor chat-prose nodrag nopan"
      contentEditable
      suppressContentEditableWarning
      role="textbox"
      aria-multiline
      onBlur={() => onDone(writeRich(readRich(field.current!)))}
      onPaste={(event) => {
        event.preventDefault()
        const pasted = event.clipboardData.getData('text/plain')
        // An address pasted over chosen words links them.
        const link = WEB_ADDRESS.test(pasted.trim()) && !getSelection()!.isCollapsed
        document.execCommand(link ? 'createLink' : 'insertText', false, link ? pasted.trim() : pasted)
      }}
      onClick={(event) => {
        const item = event.target instanceof HTMLElement && event.target.matches('li[data-checked]') ? event.target : null
        if (item && event.clientX < item.getBoundingClientRect().left) item.dataset.checked = String(item.dataset.checked !== 'true')
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') event.currentTarget.blur()
      }}
    />
  )
}

// Markdown the formatted editor cannot hold is typed as its source.
export function SourceEditor({ ref, initial, onDone }: { ref: Ref<TextEditorHandle>; initial: string; onDone: (value: string) => void }) {
  const [value, setValue] = useState(initial)
  const field = useRef<HTMLTextAreaElement>(null)
  const selection = useRef<[number, number] | null>(null)
  useImperativeHandle(ref, () => ({
    format(kind) {
      if (kind === 'list') {
        setValue(toggleList)
        return
      }
      if (kind === 'numbered' || kind === 'checklist') return
      const next = toggleMarkAt(value, field.current!.selectionStart, field.current!.selectionEnd, SOURCE_MARKS[kind])
      selection.current = [next.start, next.end]
      setValue(next.text)
    },
  }), [value])
  useLayoutEffect(() => {
    if (!selection.current) return
    field.current!.setSelectionRange(...selection.current)
    selection.current = null
  }, [value])
  useFocusWhenShown(field)
  return (
    <textarea
      ref={field}
      className="canvas-text-editor nodrag nopan"
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => onDone(value)}
      onKeyDown={(event) => {
        if (event.key === 'Escape') event.currentTarget.blur()
      }}
    />
  )
}
