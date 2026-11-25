import { createContext, useContext, useMemo, type ReactNode } from 'react'
import type { FileReference } from '@shared/fileReader'

const FileLinks = createContext<{
  sessionId: string
  cloudRunId?: string
  documentPath?: string
  onOpen: (file: FileReference) => void
} | null>(null)

export function FileReaderLinkProvider({ children, ...links }: {
  sessionId: string
  cloudRunId?: string
  documentPath?: string
  onOpen: (file: FileReference) => void
  children: ReactNode
}) {
  const { sessionId, cloudRunId, documentPath, onOpen } = links
  const value = useMemo(() => ({ sessionId, cloudRunId, documentPath, onOpen }), [sessionId, cloudRunId, documentPath, onOpen])
  return <FileLinks value={value}>{children}</FileLinks>
}

export const useFileLinks = () => useContext(FileLinks)
