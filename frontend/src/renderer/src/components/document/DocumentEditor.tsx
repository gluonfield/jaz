import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { EditorView } from '@tiptap/pm/view'
import { EditorContent, NodeViewWrapper, ReactNodeViewRenderer, useEditor, type ReactNodeViewProps } from '@tiptap/react'
import { FileText, Pencil, X } from 'lucide-react'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { FileReaderLinkProvider, RenderedMarkdown, usePreviewLink } from '@/components/session/MessageMarkdown'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { saveSessionFile, sessionFileQuery } from '@/lib/api/sessions'
import type { SessionFileRead } from '@/lib/api/types'
import { SAVE_LABEL, useDocumentSave, type DocumentVersion } from '@/lib/hooks/useDocumentSave'
import { headingAnchors, markdownDocumentTitle } from '@/lib/markdownDocuments'
import { isPlainClick, markdownLinkTarget } from '@/lib/markdownLinks'
import type { FileReference } from '@shared/fileReader'
import { documentKit, replaceSource } from './documentKit'

export type EditableDocument = SessionFileRead & { version: string }

// A document can be edited when all of it was read along with its version.
export function isEditableDocument(file: SessionFileRead): file is EditableDocument {
  return Boolean(file.version) && !file.truncated && !file.binary
}

const versionOf = (file: EditableDocument): DocumentVersion => ({ content: file.content ?? '', version: file.version })

const ArtifactPreviewContext = createContext(false)

const extensions = documentKit(() => ReactNodeViewRenderer(SourceView))

export function DocumentHeader({ title, onClose, children }: { title: string; onClose: () => void; children?: ReactNode }) {
  return (
    <header className="flex h-11 shrink-0 items-center gap-2 border-b border-border px-3">
      <FileText size={15} className="shrink-0 text-ink-3" aria-hidden />
      <p className="min-w-0 flex-1 truncate text-[13px] font-medium">{title}</p>
      {children}
      <IconButton size="sm" aria-label="Close document" onClick={onClose}><X size={16} /></IconButton>
    </header>
  )
}

// A Markdown document that is read and edited in the same place. path is the
// one the reader asked for, which names the cached read; file is that read.
export function DocumentEditor({
  sessionId,
  path,
  file,
  active,
  onOpenFile,
  onClose,
}: {
  sessionId: string
  path: string
  file: EditableDocument
  active: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const openPreview = usePreviewLink()
  const query = sessionFileQuery(sessionId, path)
  const { state, save, touch, follow, takeTheirs, keepMine } = useDocumentSave(versionOf(file), {
    read: async () => versionOf((await queryClient.fetchQuery({ ...query, staleTime: 0 })) as EditableDocument),
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
  const openLink = (view: EditorView, link: HTMLAnchorElement) => {
    const target = markdownLinkTarget(link.getAttribute('href') ?? '', link.textContent, { sessionId, documentPath: file.path })
    switch (target?.kind) {
      case 'canvas':
        return void navigate({ to: '/sessions/$sessionId', params: { sessionId: target.sessionId }, search: { page: target.page, item: target.item }, state: () => ({ canvasNavigation: crypto.randomUUID() }) })
      case 'anchor':
        return headingElement(view, target.id)?.scrollIntoView({ block: 'start' })
      case 'file':
        return onOpenFile(target.file)
      case 'url':
        return openPreview ? openPreview(target.url) : void window.open(target.url, '_blank', 'noreferrer')
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
        if (!link || !isPlainClick(event)) return false
        openLink(view, link)
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
    <div className="flex h-full min-h-0 flex-col">
      <DocumentHeader title={markdownDocumentTitle(file.content ?? '', path)} onClose={onClose}>
        <span role="status" className={`shrink-0 text-[12px] ${state === 'saving' ? 'text-ink-3' : 'text-danger'}`}>{SAVE_LABEL[state]}</span>
        {state === 'failed' ? <Button size="sm" variant="ghost" onClick={() => void save()}>Retry</Button> : null}
      </DocumentHeader>
      {state === 'conflict' ? (
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border bg-surface px-3 py-2">
          <span className="text-[13px] text-ink-2">This document changed while you were editing.</span>
          <span className="flex shrink-0 gap-2">
            <Button size="sm" variant="ghost" onClick={() => void takeTheirs()}>Use theirs</Button>
            <Button size="sm" variant="secondary" onClick={() => void keepMine()}>Keep mine</Button>
          </span>
        </div>
      ) : null}
      <FileReaderLinkProvider sessionId={sessionId} documentPath={file.path} onOpen={onOpenFile}>
        <ArtifactPreviewContext value={!active}>
          <EditorContent editor={editor} className="document-editor scrollbar-quiet min-h-0 flex-1 overflow-auto bg-bg" />
        </ArtifactPreviewContext>
      </FileReaderLinkProvider>
    </div>
  )
}

// A contents link names its heading the way the read-only renderer does.
function headingElement(view: EditorView, id: string): HTMLElement | null {
  const anchor = headingAnchors()
  let found = -1
  view.state.doc.descendants((node, pos) => {
    if (found < 0 && node.type.name === 'heading' && anchor(node.textContent) === id) found = pos
  })
  return found < 0 ? null : (view.nodeDOM(found) as HTMLElement)
}

// Markdown the editor keeps as source is drawn by the read-only renderer, and
// edited as the text it is: whatever is written replaces it when focus leaves,
// and Escape leaves it as it was.
function SourceView({ node, editor, getPos }: ReactNodeViewProps) {
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
            if (event.target.value !== source && pos !== undefined) replaceSource(editor, pos, event.target.value)
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
