import type { Session } from '@/lib/api/types'
import type { PlanSurface } from '@/lib/planSurface'
import type { FileReference } from '../../../../shared/fileReader'
import { CODE_DIFF_PANEL_WIDTH, CodeDiffPanel } from './CodeDiffPanel'
import { FILE_READER_PANEL_WIDTH, FileReaderPanel } from './FileReaderPanel'
import { OVERVIEW_PANEL_WIDTH, OverviewPanel } from './OverviewPanel'
import { PREVIEW_PANEL_WIDTH, PreviewPanel } from './PreviewPanel'

export type SidePanelView = 'overview' | 'diff' | 'preview' | 'file'

export const SIDE_PANEL_WIDTHS: Record<SidePanelView, number> = {
  overview: OVERVIEW_PANEL_WIDTH,
  diff: CODE_DIFF_PANEL_WIDTH,
  preview: PREVIEW_PANEL_WIDTH,
  file: FILE_READER_PANEL_WIDTH,
}

export function SidePanel({
  session,
  plan,
  working,
  visible,
  view,
  previewUrl,
  fileRef,
  onPreviewUrlChange,
  onOpenFile,
  onClose,
}: {
  session: Session
  plan?: PlanSurface
  working: boolean
  visible: boolean
  view: SidePanelView
  previewUrl: string
  fileRef: FileReference | null
  onPreviewUrlChange: (url: string) => void
  onOpenFile: (file: FileReference) => void
  onClose: () => void
}) {
  switch (view) {
    case 'diff':
      return <CodeDiffPanel session={session} visible={visible} onClose={onClose} />
    case 'preview':
      return <PreviewPanel url={previewUrl} onUrlChange={onPreviewUrlChange} onClose={onClose} />
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
      return <OverviewPanel session={session} plan={plan} working={working} />
  }
}
