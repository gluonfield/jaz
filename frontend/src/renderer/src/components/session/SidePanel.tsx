import type { Attachment, Session } from '@/lib/api/types'
import type { BrowserAnnotation, SendMessageOptions } from '@/lib/sendMessage'
import type { TaskSurface } from '@/lib/taskSurface'
import type { FileReference } from '../../../../shared/fileReader'
import { CODE_DIFF_PANEL_WIDTH, CodeDiffPanel } from './CodeDiffPanel'
import { FILE_READER_PANEL_WIDTH, FileReaderPanel } from './FileReaderPanel'
import { OVERVIEW_PANEL_WIDTH, OverviewPanel } from './OverviewPanel'
import { PREVIEW_PANEL_WIDTH, PreviewPanel } from './PreviewPanel'
import { TERMINAL_PANEL_WIDTH, TerminalPanel } from './TerminalPanel'

export type SidePanelView = 'overview' | 'diff' | 'preview' | 'terminal' | 'file'

export const SIDE_PANEL_WIDTHS: Record<SidePanelView, number> = {
  overview: OVERVIEW_PANEL_WIDTH,
  diff: CODE_DIFF_PANEL_WIDTH,
  preview: PREVIEW_PANEL_WIDTH,
  terminal: TERMINAL_PANEL_WIDTH,
  file: FILE_READER_PANEL_WIDTH,
}

export function SidePanel({
  session,
  progress,
  working,
  visible,
  view,
  previewUrl,
  fileRef,
  onPreviewUrlChange,
  onOpenFile,
  onAddBrowserAnnotation,
  onUploadAttachment,
  onSend,
  onClose,
}: {
  session: Session
  progress?: TaskSurface
  working: boolean
  visible: boolean
  view: SidePanelView
  previewUrl: string
  fileRef: FileReference | null
  onPreviewUrlChange: (url: string) => void
  onOpenFile: (file: FileReference) => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onSend: (text: string, options?: SendMessageOptions) => void
  onClose: () => void
}) {
  switch (view) {
    case 'diff':
      return <CodeDiffPanel session={session} visible={visible} onClose={onClose} />
    case 'preview':
      return (
        <PreviewPanel
          url={previewUrl}
          onUrlChange={onPreviewUrlChange}
          onAddBrowserAnnotation={onAddBrowserAnnotation}
          onUploadAttachment={onUploadAttachment}
          onClose={onClose}
        />
      )
    case 'terminal':
      return <TerminalPanel session={session} visible={visible} onClose={onClose} />
    case 'file':
      return (
        <FileReaderPanel
          session={session}
          fileRef={fileRef}
          visible={visible}
          onOpenFile={onOpenFile}
          onClose={onClose}
        />
      )
    default:
      return (
        <OverviewPanel
          session={session}
          progress={progress}
          working={working}
          onSend={onSend}
        />
      )
  }
}
