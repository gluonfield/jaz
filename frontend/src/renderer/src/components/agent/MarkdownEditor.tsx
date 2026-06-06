import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { Prec } from '@codemirror/state'
import { EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view'
import { tags } from '@lezer/highlight'
import { useEffect, useRef } from 'react'

const theme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: '0.875rem',
    backgroundColor: 'transparent',
  },
  '.cm-content': {
    fontFamily: "'JetBrains Mono Variable', ui-monospace, monospace",
    lineHeight: '1.7',
    padding: '16px 20px',
    caretColor: 'oklch(0.27 0.015 110)',
  },
  '.cm-line': { padding: '0' },
  '&.cm-focused': { outline: 'none' },
  '.cm-activeLine': { backgroundColor: 'oklch(0.972 0.004 110 / 60%)' },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
    backgroundColor: 'oklch(0.93 0.04 110)',
  },
  '.cm-cursor': { borderLeftColor: 'oklch(0.27 0.015 110)' },
  '.cm-scroller': { overflow: 'auto' },
  '.cm-placeholder': { color: 'oklch(0.56 0.012 110)' },
})

const highlight = HighlightStyle.define([
  { tag: tags.heading, fontWeight: '600', color: 'oklch(0.27 0.015 110)' },
  { tag: tags.strong, fontWeight: '600' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.link, color: 'oklch(0.55 0.12 110)' },
  { tag: tags.url, color: 'oklch(0.55 0.12 110)' },
  { tag: tags.monospace, color: 'oklch(0.52 0.12 55)' },
  { tag: tags.quote, color: 'oklch(0.5 0.014 110)' },
  { tag: tags.list, color: 'oklch(0.5 0.014 110)' },
])

export function MarkdownEditor({
  initialValue,
  placeholder,
  onChange,
  onSave,
}: {
  initialValue: string
  placeholder?: string
  onChange: (doc: string) => void
  onSave: () => void
}) {
  const hostRef = useRef<HTMLDivElement>(null)
  const onChangeRef = useRef(onChange)
  const onSaveRef = useRef(onSave)
  onChangeRef.current = onChange
  onSaveRef.current = onSave

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const view = new EditorView({
      parent: host,
      doc: initialValue,
      extensions: [
        history(),
        Prec.high(
          keymap.of([
            {
              key: 'Mod-s',
              run: () => {
                onSaveRef.current()
                return true
              },
            },
          ]),
        ),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        EditorView.lineWrapping,
        markdown(),
        syntaxHighlighting(highlight),
        theme,
        ...(placeholder ? [cmPlaceholder(placeholder)] : []),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) onChangeRef.current(update.state.doc.toString())
        }),
      ],
    })

    return () => view.destroy()
    // The editor owns its document after mount; remount (via key) to reset.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return <div ref={hostRef} className="h-full min-h-0 select-text" />
}
