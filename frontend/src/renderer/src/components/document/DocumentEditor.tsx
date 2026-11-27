import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { EditorView } from '@tiptap/pm/view'
import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react'
import { FileText, Share2, X } from 'lucide-react'
import { useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { PublishAppModal } from '@/components/apps/PublishAppModal'
import { FileReaderLinkProvider, usePreviewLink } from '@/components/session/MessageMarkdown'
import { SidePanelCloseGuard } from '@/components/session/SidePanelState'
import { FileActions } from '@/components/session/FileActions'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { useToast } from '@/components/ui/toast'
import { createSessionFile, effectiveCloudRunId, saveSessionFile, sessionFileQuery } from '@/lib/api/sessions'
import type { SessionFileRead } from '@/lib/api/types'
import { SAVE_LABEL, useDocumentSave, type DocumentVersion } from '@/lib/hooks/useDocumentSave'
import { headingAnchors, markdownDocumentTitle } from '@/lib/markdownDocuments'
import { isPlainClick, markdownLinkTarget } from '@/lib/markdownLinks'
import { useFileLocation } from '@/lib/hooks/useFileLocation'
import type { FileLocation, FileReference } from '@shared/fileReader'
import { documentKit } from './documentKit'
import { DocumentToolbar } from './DocumentToolbar'
import { SelectionMenu } from './SelectionMenu'
import { SlashMenu } from './SlashMenu'
import { ArtifactPreviewContext, SourceView } from './SourceView'
import { TableMenu } from './TableMenu'

export type EditableDocument = SessionFileRead & { version: string }

// A document can be edited when all of it was read along with its version.
export function isEditableDocument(file: SessionFileRead): file is EditableDocument {
  return Boolean(file.version) && !file.truncated && !file.binary && !file.read_only
}

const versionOf = (file: EditableDocument): DocumentVersion => ({ content: file.content ?? '', version: file.version })

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
  cloudRunId,
  path,
  file,
  location,
  active,
  onOpenFile,
  onClose,
}: {
  sessionId: string
  cloudRunId?: string
  path: string
  file: EditableDocument
  location?: FileLocation
  active: boolean
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const openPreview = usePreviewLink()
  const toast = useToast()
  const [publishOpen, setPublishOpen] = useState(false)
  const runId = effectiveCloudRunId(sessionId, cloudRunId)
  const readOnly = Boolean(runId) || Boolean(file.read_only)
  const query = sessionFileQuery(sessionId, path, runId)
  const { state, save, unsaved, touch, follow, takeTheirs, keepMine } = useDocumentSave(versionOf(file), {
    read: async () => versionOf((await queryClient.fetchQuery({ ...query, staleTime: 0 })) as EditableDocument),
    // A read begun before the save would put the older text back in the cache.
    write: async (content, version) => {
      if (readOnly) return { content, version }
      const saved = { content, version: await saveSessionFile(sessionId, file.path, content, version) }
      await queryClient.cancelQueries(query)
      queryClient.setQueryData(query.queryKey, (current) => current && { ...current, ...saved })
      return saved
    },
    conflict: 412,
    delay: 2000,
  })
  const registerCloseGuard = useContext(SidePanelCloseGuard)
  const canClose = useCallback(async () => !(await unsaved()) || window.confirm('Discard your edits to this document?'), [unsaved])
  useEffect(() => registerCloseGuard?.(canClose), [registerCloseGuard, canClose])
  const close = async () => {
    if (await canClose()) onClose()
  }
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
  const saveImage = async (beside: string, image: File) => {
    if (readOnly) return false
    try {
      await createSessionFile(sessionId, file.path.replace(/[^/\\]*$/, beside), image)
      return true
    } catch {
      toast(`Could not add ${image.name}`, 'danger')
      return false
    }
  }
  const [extensions] = useState(() => documentKit({ sourceView: () => ReactNodeViewRenderer(SourceView), saveImage }))
  const editor = useEditor({
    extensions,
    editable: !readOnly,
    content: file.content,
    contentType: 'markdown',
    editorProps: {
      attributes: { class: 'file-prose outline-none', spellcheck: 'false', 'aria-label': 'Document' },
      handleDOMEvents: {
        // Opening a file hides this document, and the browser follows a link
        // that is no longer editable, so the click is cancelled here.
        click: (view, event) => {
          const link = (event.target as HTMLElement).closest('a')
          if (!link || !isPlainClick(event)) return false
          event.preventDefault()
          openLink(view, link)
          return true
        },
      },
    },
    onUpdate: ({ editor }) => {
      if (!readOnly) touch(editor)
    },
    onBlur: () => {
      if (!readOnly) void save()
    },
  })
  useEffect(() => {
    if (editor) follow(editor, versionOf(file))
  }, [editor, follow, file])

  const lineText = useMemo(() => {
    if (!editor || !location?.line || location.text) return
    const source = (file.content ?? '').split('\n')[location.line - 1] ?? ''
    const parsed = editor.markdown?.parse(source)
    return parsed ? editor.schema.nodeFromJSON(parsed).textContent : undefined
  }, [editor, location, file.content])
  useFileLocation(editor?.view.dom ?? null, location, file.content ?? '', active, lineText)

  return (
    <div className="relative flex h-full min-h-0 flex-col">
      <DocumentHeader title={markdownDocumentTitle(file.content ?? '', path)} onClose={() => void close()}>
        {readOnly ? (
          <span className="shrink-0 text-[12px] text-ink-3">Read only</span>
        ) : (
          <>
            <span role="status" className={`shrink-0 text-[12px] ${state === 'saving' ? 'text-ink-3' : 'text-danger'}`}>{SAVE_LABEL[state]}</span>
            {state === 'failed' ? <Button size="sm" variant="ghost" onClick={() => void save()}>Retry</Button> : null}
          </>
        )}
        <Button size="sm" variant="ghost" onClick={() => setPublishOpen(true)} title="Publish document as Jaz App">
          <Share2 size={12} />
          Publish
        </Button>
        <FileActions sessionId={sessionId} cloudRunId={runId} path={file.path} beforeDownload={readOnly ? undefined : async () => !(await unsaved())} />
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
      <FileReaderLinkProvider sessionId={sessionId} cloudRunId={runId} documentPath={file.path} onOpen={onOpenFile}>
        <ArtifactPreviewContext value={!active}>
          <EditorContent editor={editor} className="document-editor scrollbar-quiet min-h-0 flex-1 overflow-auto bg-bg" />
        </ArtifactPreviewContext>
      </FileReaderLinkProvider>
      {editor && !readOnly ? (
        <>
          <SlashMenu editor={editor} />
          <SelectionMenu editor={editor} />
          <TableMenu editor={editor} />
          <DocumentToolbar editor={editor} />
        </>
      ) : null}
      {publishOpen ? (
        <PublishAppModal
          open
          onClose={() => setPublishOpen(false)}
          initialTitle={markdownDocumentTitle(file.content ?? '', path)}
          kind="document"
          entry="index.md"
          markdown={file.content ?? ''}
          sourceThreadId={sessionId}
        />
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
