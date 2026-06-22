import { useCallback, useEffect, useRef } from 'react'
import { readSessionFile } from '@/lib/api/sessions'
import { shouldPreviewFileReference, type FileReference } from '../../../../shared/fileReader'

export function useFileReferencePreview({
  sessionId,
  onOpenFile,
  onOpenPreview,
  onPreviewError,
}: {
  sessionId: string
  onOpenFile: (file: FileReference) => void
  onOpenPreview: (url: string) => void
  onPreviewError: (message: string) => void
}) {
  const htmlPreviewURLRef = useRef('')

  useEffect(() => () => revokeObjectURLRef(htmlPreviewURLRef), [])

  const openHTMLFilePreview = useCallback(
    async (file: FileReference) => {
      try {
        const data = await readSessionFile(sessionId, file.path)
        if (data.binary || data.content === undefined) {
          throw new Error('HTML file has no readable text content')
        }
        revokeObjectURLRef(htmlPreviewURLRef)
        htmlPreviewURLRef.current = URL.createObjectURL(new Blob([data.content], { type: 'text/html;charset=utf-8' }))
        onOpenPreview(htmlPreviewURLRef.current)
      } catch (err) {
        onPreviewError(err instanceof Error ? err.message : String(err))
      }
    },
    [onOpenPreview, onPreviewError, sessionId],
  )

  return useCallback(
    (file: FileReference) => {
      if (shouldPreviewFileReference(file)) {
        void openHTMLFilePreview(file)
        return
      }
      onOpenFile(file)
    },
    [onOpenFile, openHTMLFilePreview],
  )
}

function revokeObjectURLRef(ref: { current: string }) {
  if (!ref.current) return
  URL.revokeObjectURL(ref.current)
  ref.current = ''
}
