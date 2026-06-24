import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { Prec } from '@codemirror/state'
import { EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view'
import { tags } from '@lezer/highlight'
import { useEffect, useRef } from 'react'

// Colors reference the theme tokens (via CSS vars), so the editor follows
// light/dark without rebuilding the EditorView.
const theme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: '0.875rem',
    backgroundColor: 'transparent',
  },
  '.cm-content': {
    fontFamily: 'var(--font-mono)',
    lineHeight: '1.7',
    padding: '16px 20px',
    caretColor: 'var(--color-ink)',
  },
  '.cm-line': { padding: '0' },
  '&.cm-focused': { outline: 'none' },
  '.cm-activeLine': { backgroundColor: 'var(--color-surface-2)' },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
    backgroundColor: 'var(--color-primary-soft)',
  },
  '.cm-cursor': { borderLeftColor: 'var(--color-ink)' },
  '.cm-scroller': { overflow: 'auto' },
  '.cm-placeholder': { color: 'var(--color-ink-3)' },
})

const highlight = HighlightStyle.define([
  { tag: tags.heading, fontWeight: '600', color: 'var(--color-ink)' },
  { tag: tags.strong, fontWeight: '600' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.link, color: 'var(--color-primary)' },
  { tag: tags.url, color: 'var(--color-primary)' },
  { tag: tags.monospace, color: 'var(--color-accent-strong)' },
  { tag: tags.quote, color: 'var(--color-ink-2)' },
  { tag: tags.list, color: 'var(--color-ink-2)' },
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
