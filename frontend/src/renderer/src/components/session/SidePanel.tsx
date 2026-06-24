import type { Attachment, Session, SessionEvent } from '@/lib/api/types'
import type { BrowserAnnotation } from '@/lib/messageContext'
import type { ProviderSubagentView } from '@/lib/providerSubagents'
import type { SendMessageOptions } from '@/lib/sendMessage'
import type { SpawnedThreadView } from '@/lib/spawnedThreads'
import type { TaskSurface } from '@/lib/taskSurface'
import type { FileReference } from '../../../../shared/fileReader'
import { CODE_DIFF_PANEL_WIDTH, CodeDiffPanel } from './CodeDiffPanel'
import { FILE_READER_PANEL_WIDTH, FileReaderPanel } from './FileReaderPanel'
import { OVERVIEW_PANEL_WIDTH, OverviewPanel } from './OverviewPanel'
import { PREVIEW_PANEL_WIDTH, PreviewPanel } from './PreviewPanel'
import { SIDE_CHAT_PANEL_WIDTH, SideChatPanel } from './SideChatPanel'
import { TERMINAL_PANEL_WIDTH, TerminalPanel } from './TerminalPanel'

export type SidePanelView = 'overview' | 'diff' | 'preview' | 'terminal' | 'file' | 'side-chat'

export const SIDE_PANEL_WIDTHS: Record<SidePanelView, number> = {
  overview: OVERVIEW_PANEL_WIDTH,
  diff: CODE_DIFF_PANEL_WIDTH,
  preview: PREVIEW_PANEL_WIDTH,
  terminal: TERMINAL_PANEL_WIDTH,
  file: FILE_READER_PANEL_WIDTH,
  'side-chat': SIDE_CHAT_PANEL_WIDTH,
}

export function SidePanel({
  session,
  progress,
  subagents,
  spawnedThreads,
  working,
  visible,
  view,
  previewUrl,
  fileRef,
  sideChatAvailable,
  sideChatEvents,
  onPreviewUrlChange,
  onOpenFile,
  onAddBrowserAnnotation,
  onUploadAttachment,
  onSend,
  onSendSideChat,
  onClose,
}: {
  session: Session
  progress?: TaskSurface
  subagents: ProviderSubagentView[]
  spawnedThreads: SpawnedThreadView[]
  working: boolean
  visible: boolean
  view: SidePanelView
  previewUrl: string
  fileRef: FileReference | null
  sideChatAvailable: boolean
  sideChatEvents: SessionEvent[]
  onPreviewUrlChange: (url: string) => void
  onOpenFile: (file: FileReference) => void
  onAddBrowserAnnotation?: (annotation: BrowserAnnotation, screenshot?: Attachment) => void
  onUploadAttachment?: (file: File) => Promise<Attachment>
  onSend: (text: string, options?: SendMessageOptions) => void
  onSendSideChat: (sideChatID: string, message: string) => Promise<void>
  onClose: () => void
}) {
  switch (view) {
    case 'side-chat':
      return sideChatAvailable ? (
        <SideChatPanel
          events={sideChatEvents}
          visible={visible}
          onSend={onSendSideChat}
          onClose={onClose}
        />
      ) : (
        <OverviewPanel
          session={session}
          progress={progress}
          subagents={subagents}
          spawnedThreads={spawnedThreads}
          working={working}
          onSend={onSend}
        />
      )
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
          subagents={subagents}
          spawnedThreads={spawnedThreads}
          working={working}
          onSend={onSend}
        />
      )
  }
}
