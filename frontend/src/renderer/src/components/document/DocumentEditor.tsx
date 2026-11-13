import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { EditorView } from '@tiptap/pm/view'
import { EditorContent, NodeViewWrapper, ReactNodeViewRenderer, useEditor, type ReactNodeViewProps } from '@tiptap/react'
import { Pencil } from 'lucide-react'
import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FileReaderLinkProvider, RenderedMarkdown, isUrlLink, localFileFromLink, shouldPreviewLink, usePreviewLink } from '@/components/session/MessageMarkdown'
import { Button } from '@/components/ui/Button'
import { saveSessionFile, sessionFileQuery } from '@/lib/api/sessions'
import type { SessionFileRead } from '@/lib/api/types'
import { canvasLink } from '@/lib/canvasNavigation'
import { SAVE_LABEL, useDocumentSave, type DocumentVersion } from '@/lib/hooks/useDocumentSave'
import { headingAnchors } from '@/lib/markdownDocuments'
import type { FileReference } from '@shared/fileReader'
import { documentKit, replaceSource, SourceBlock, SourceInline } from './documentKit'

export type EditableDocument = SessionFileRead & { version: string }

// A document can be edited when all of it was read along with its version.
export function isEditableDocument(file: SessionFileRead): file is EditableDocument {
  return Boolean(file.version) && !file.truncated && !file.binary
}

const versionOf = (file: SessionFileRead): DocumentVersion => ({ content: file.content ?? '', version: file.version! })

const ArtifactPreviewContext = createContext(false)

const sourceView = { addNodeView: () => ReactNodeViewRenderer(SourceView) }
const extensions = documentKit.map((extension) =>
  extension === SourceBlock ? SourceBlock.extend(sourceView) : extension === SourceInline ? SourceInline.extend(sourceView) : extension,
)

// A Markdown document that is read and edited in the same place. path is the
// one the reader asked for, which names the cached read; file is that read.
export function DocumentEditor({
  sessionId,
  path,
  file,
  active,
  status,
  onOpenFile,
}: {
  sessionId: string
  path: string
  file: EditableDocument
  active: boolean
  status: HTMLElement | null
  onOpenFile: (file: FileReference) => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const openPreview = usePreviewLink()
  const query = sessionFileQuery(sessionId, path)
  const { state, save, touch, follow, takeTheirs, keepMine } = useDocumentSave(versionOf(file), {
    read: async () => versionOf(await queryClient.fetchQuery({ ...query, staleTime: 0 })),
    // A read begun before the save would put the older text back in the cache.
    write: async (content, version) => {
      const saved = { content, version: await saveSessionFile(sessionId, file.path, content, version) }
      await queryClient.cancelQueries(query)
      queryClient.setQueryData(query.queryKey, (current) => current && { ...current, ...saved })
      return saved
    },
    conflict: 412,
    delay: 2000,
  })
  const openLink = (view: EditorView, href: string, label: string) => {
    const canvas = canvasLink(href, sessionId)
    const local = localFileFromLink(href, label, file.path)
    if (canvas) {
      void navigate({ to: '/sessions/$sessionId', params: { sessionId: canvas.sessionId }, search: { page: canvas.page, item: canvas.item }, state: () => ({ canvasNavigation: crypto.randomUUID() }) })
    } else if (href.startsWith('#')) {
      headingElement(view, href.slice(1))?.scrollIntoView({ block: 'start' })
    } else if (local) {
      onOpenFile(local)
    } else if (isUrlLink(href) && openPreview) {
      openPreview(href)
    } else if (isUrlLink(href)) {
      window.open(href, '_blank', 'noreferrer')
    }
  }
  const editor = useEditor({
    extensions,
    content: file.content,
    contentType: 'markdown',
    editorProps: {
      attributes: { class: 'file-prose outline-none', spellcheck: 'false', 'aria-label': 'Document' },
      handleClick: (view, _pos, event) => {
        const link = (event.target as HTMLElement).closest('a')
        const href = link?.getAttribute('href')
        if (!link || !href || !shouldPreviewLink(event)) return false
        openLink(view, href, link.textContent)
        return true
      },
    },
    onUpdate: ({ editor }) => touch(editor),
    onBlur: () => void save(),
  })
  useEffect(() => {
    if (editor) follow(editor, versionOf(file))
  }, [editor, follow, file])

  return (
    <FileReaderLinkProvider sessionId={sessionId} documentPath={file.path} onOpen={onOpenFile}>
      <ArtifactPreviewContext value={!active}>
        {status && createPortal(
          <>
            <span role="status" className={state === 'saving' ? 'text-ink-3' : 'text-danger'}>{SAVE_LABEL[state]}</span>
            {state === 'failed' ? <Button size="sm" variant="ghost" className="ml-1" onClick={() => void save()}>Retry</Button> : null}
          </>,
          status,
        )}
        {state === 'conflict' ? (
          <div className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border bg-surface px-3 py-2">
            <span className="text-[13px] text-ink-2">This document changed while you were editing.</span>
            <span className="flex shrink-0 gap-2">
              <Button size="sm" variant="ghost" onClick={() => void takeTheirs()}>Use theirs</Button>
              <Button size="sm" variant="secondary" onClick={() => void keepMine()}>Keep mine</Button>
            </span>
          </div>
        ) : null}
        <EditorContent editor={editor} className="document-editor" />
      </ArtifactPreviewContext>
    </FileReaderLinkProvider>
  )
}

// A contents link names its heading the way the read-only renderer does.
function headingElement(view: EditorView, encoded: string): HTMLElement | null {
  let id: string
  try {
    id = decodeURIComponent(encoded)
  } catch {
    return null
  }
  const anchor = headingAnchors()
  let found = -1
  view.state.doc.descendants((node, pos) => {
    if (found < 0 && node.type.name === 'heading' && anchor(node.textContent) === id) found = pos
  })
  return found < 0 ? null : (view.nodeDOM(found) as HTMLElement)
}

// Markdown the editor keeps as source is drawn by the read-only renderer, and
// edited as the text it is: whatever is written replaces it when focus leaves.
function SourceView({ node, editor, getPos }: ReactNodeViewProps) {
  const source = node.attrs.source as string
  const [draft, setDraft] = useState(node.attrs.editing ? source : null)
  const cancelled = useRef(false)
  const preview = useContext(ArtifactPreviewContext)
  return (
    <NodeViewWrapper as={node.isInline ? 'span' : 'div'} className="document-source" data-source={source}>
      {draft === null ? (
        <>
          <RenderedMarkdown text={source} className="document-source-view" artifactPreview={preview} />
          <button type="button" aria-label="Edit as Markdown" title="Edit as Markdown" onClick={() => setDraft(source)}>
            <Pencil size={12} aria-hidden />
          </button>
        </>
      ) : (
        <textarea
          autoFocus
          aria-label="Markdown source"
          spellCheck={false}
          rows={draft.split('\n').length}
          cols={node.isInline ? Math.max(12, draft.length + 2) : undefined}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onFocus={(event) => {
            const start = node.attrs.editing ? source.indexOf('\n') + 1 : source.length
            event.target.setSelectionRange(start, start)
          }}
          onBlur={() => {
            setDraft(null)
            const pos = getPos()
            if (!cancelled.current && draft !== source && pos !== undefined) replaceSource(editor, pos, draft)
            cancelled.current = false
          }}
          onKeyDown={(event) => {
            cancelled.current = event.key === 'Escape'
            if (cancelled.current || (event.key === 'Enter' && (node.isInline || event.metaKey || event.ctrlKey))) {
              event.preventDefault()
              editor.commands.focus()
            }
          }}
        />
      )}
    </NodeViewWrapper>
  )
}
