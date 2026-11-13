import { useQueryClient } from '@tanstack/react-query'
import { useBlocker, useNavigate } from '@tanstack/react-router'
import type { EditorView } from '@tiptap/pm/view'
import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react'
import { FileText, X } from 'lucide-react'
import { useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { FileReaderLinkProvider, usePreviewLink } from '@/components/session/MessageMarkdown'
import { SidePanelCloseGuard } from '@/components/session/SidePanelState'
import { Button } from '@/components/ui/Button'
import { clipboardFiles } from '@/components/ui/fileTransfer'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { createSessionFile, saveSessionFile, sessionFileQuery } from '@/lib/api/sessions'
import type { SessionFileRead } from '@/lib/api/types'
import { SAVE_LABEL, useDocumentSave, type DocumentVersion } from '@/lib/hooks/useDocumentSave'
import { headingAnchors, markdownDocumentTitle } from '@/lib/markdownDocuments'
import { isPlainClick, markdownLinkTarget } from '@/lib/markdownLinks'
import type { FileReference } from '@shared/fileReader'
import { documentCommands } from './documentCommands'
import { documentKit } from './documentKit'
import { DocumentOutline } from './DocumentOutline'
import { DocumentToolbar } from './DocumentToolbar'
import { SelectionMenu } from './SelectionMenu'
import { SlashMenu } from './SlashMenu'
import { ArtifactPreviewContext, SourceView } from './SourceView'
import { TableMenu } from './TableMenu'

export type EditableDocument = SessionFileRead & { version: string }

// A document can be edited when all of it was read along with its version.
export function isEditableDocument(file: SessionFileRead): file is EditableDocument {
  return Boolean(file.version) && !file.truncated && !file.binary
}

const versionOf = (file: EditableDocument): DocumentVersion => ({ content: file.content ?? '', version: file.version })

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
  const toast = useToast()
  const imagePicker = useRef<HTMLInputElement>(null)
  const commands = useMemo(() => documentCommands(() => imagePicker.current?.click()), [])
  const query = sessionFileQuery(sessionId, path)
  const { state, edited, save, touch, follow, takeTheirs, keepMine } = useDocumentSave(versionOf(file), {
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
  const registerCloseGuard = useContext(SidePanelCloseGuard)
  const canClose = useCallback(async () => {
    await save()
    return !edited.current || window.confirm('Discard your edits to this document?')
  }, [save, edited])
  useEffect(() => registerCloseGuard?.(canClose), [registerCloseGuard, canClose])
  useBlocker({
    shouldBlockFn: async () => {
      await save()
      return edited.current
    },
    enableBeforeUnload: false,
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
  // A picture is saved in an images folder beside the document and shown from there.
  const addImages = (view: EditorView, files: File[]) => {
    const images = files.filter((image) => image.type.startsWith('image/'))
    for (const image of images) {
      const path = `images/${crypto.randomUUID().slice(0, 8)}-${image.name.replace(/[^\w.-]+/g, '-')}`
      const source = `![${image.name.replace(/\.\w+$/, '').replace(/[[\]]/g, '')}](${path})`
      createSessionFile(sessionId, file.path.replace(/[^/\\]*$/, path), image).then(
        () => view.isDestroyed || view.dispatch(view.state.tr.replaceSelectionWith(view.state.schema.nodes.sourceInline.create({ source }))),
        () => toast(`Could not add ${image.name}`, 'danger'),
      )
    }
    return images.length > 0
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
      handlePaste: (view, event) => !event.clipboardData?.types.includes('text/plain') && addImages(view, event.clipboardData ? clipboardFiles(event.clipboardData) : []),
    },
    onUpdate: ({ editor }) => touch(editor),
    onBlur: () => void save(),
  })
  useEffect(() => {
    if (editor) follow(editor, versionOf(file))
  }, [editor, follow, file])

  return (
    <div className="relative flex h-full min-h-0 flex-col">
      <DocumentHeader title={markdownDocumentTitle(file.content ?? '', path)} onClose={async () => { if (await canClose()) onClose() }}>
        <span role="status" className={`shrink-0 text-[12px] ${state === 'saving' ? 'text-ink-3' : 'text-danger'}`}>{SAVE_LABEL[state]}</span>
        {state === 'failed' ? <Button size="sm" variant="ghost" onClick={() => void save()}>Retry</Button> : null}
        {editor ? <DocumentOutline editor={editor} /> : null}
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
      {editor ? (
        <>
          <SlashMenu editor={editor} groups={commands.slash} />
          <SelectionMenu editor={editor} />
          <TableMenu editor={editor} />
          <DocumentToolbar editor={editor} commands={commands} />
          <input
            ref={imagePicker}
            type="file"
            accept="image/*"
            multiple
            hidden
            onChange={(event) => {
              addImages(editor.view, [...(event.target.files ?? [])])
              event.target.value = ''
            }}
          />
        </>
      ) : null}
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
