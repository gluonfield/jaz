import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { sessionFileQuery } from '@/lib/api/sessions'
import { markdownDocumentTitle } from '@/lib/markdownDocuments'
import type { FileReference } from '@shared/fileReader'

export function MarkdownDocumentCard({ sessionId, file, label, onOpen, renderContent }: {
  sessionId: string
  file: FileReference
  label: string
  onOpen: (file: FileReference) => void
  renderContent: (content: string, path: string) => ReactNode
}) {
  const query = useQuery(sessionFileQuery(sessionId, file.path))
  const content = query.data?.binary ? '' : query.data?.content ?? ''
  const title = markdownDocumentTitle(content, file.path, label)
  return (
    <div className="markdown-document-card">
      <div className="markdown-document-preview" aria-hidden inert>
        {query.isPending ? (
          <p className="px-5 py-6 text-sm text-ink-3">Loading document…</p>
        ) : query.isError || query.data?.binary ? (
          <p className="px-5 py-6 text-sm text-ink-3">Preview unavailable. Open the document to try again.</p>
        ) : content ? renderContent(content.slice(0, 3000), query.data?.path ?? file.path) : (
          <p className="px-5 py-6 text-sm text-ink-3">Empty document.</p>
        )}
      </div>
      <button type="button" aria-label={`Open ${title}`} className="absolute inset-0 rounded-[inherit] outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={() => onOpen(file)} />
    </div>
  )
}
