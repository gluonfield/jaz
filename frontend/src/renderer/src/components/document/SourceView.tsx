import { NodeViewWrapper, type ReactNodeViewProps } from '@tiptap/react'
import { Pencil } from 'lucide-react'
import { createContext, useContext, useState } from 'react'
import { RenderedMarkdown } from '@/components/session/MessageMarkdown'
import { replaceSource } from './documentKit'

// Whether artifacts in the document draw as previews, as in a panel behind another.
export const ArtifactPreviewContext = createContext(false)

// Markdown the editor keeps as source is drawn by the read-only renderer, and
// edited as the text it is: whatever is written replaces it when focus leaves,
// and Escape leaves it as it was. A block put in and left empty goes again.
export function SourceView({ node, editor, getPos }: ReactNodeViewProps) {
  const source = node.attrs.source as string
  const [editing, setEditing] = useState(node.attrs.editing as boolean)
  const preview = useContext(ArtifactPreviewContext)
  return (
    <NodeViewWrapper as={node.isInline ? 'span' : 'div'} className="document-source" data-source={source}>
      {editing ? (
        <textarea
          autoFocus
          aria-label="Markdown source"
          spellCheck={false}
          defaultValue={source}
          rows={source.split('\n').length}
          onInput={(event) => {
            event.currentTarget.rows = event.currentTarget.value.split('\n').length
          }}
          onFocus={(event) => {
            const start = node.attrs.editing ? source.indexOf('\n') + 1 : source.length
            event.target.setSelectionRange(start, start)
          }}
          onBlur={(event) => {
            setEditing(false)
            const pos = getPos()
            if (pos === undefined) return
            if (event.target.value !== source) replaceSource(editor, pos, event.target.value)
            else if (node.attrs.editing) replaceSource(editor, pos, '')
          }}
          onKeyDown={(event) => {
            if (event.key === 'Escape') event.currentTarget.value = source
            if (event.key === 'Escape' || (event.key === 'Enter' && (node.isInline || event.metaKey || event.ctrlKey))) {
              event.preventDefault()
              editor.commands.focus()
            }
          }}
        />
      ) : (
        <>
          <RenderedMarkdown text={source} className="document-source-view" artifactPreview={preview} />
          <button type="button" aria-label="Edit as Markdown" title="Edit as Markdown" onClick={() => setEditing(true)}>
            <Pencil size={12} aria-hidden />
          </button>
        </>
      )}
    </NodeViewWrapper>
  )
}
