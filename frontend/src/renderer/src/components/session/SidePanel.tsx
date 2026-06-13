import type { Session } from '@/lib/api/types'
import type { PlanSurface } from '@/lib/planSurface'
import { CODE_DIFF_PANEL_WIDTH, CodeDiffPanel } from './CodeDiffPanel'
import { OVERVIEW_PANEL_WIDTH, OverviewPanel } from './OverviewPanel'
import { PREVIEW_PANEL_WIDTH, PreviewPanel } from './PreviewPanel'

export type SidePanelView = 'overview' | 'diff' | 'preview'

export const SIDE_PANEL_WIDTHS: Record<SidePanelView, number> = {
  overview: OVERVIEW_PANEL_WIDTH,
  diff: CODE_DIFF_PANEL_WIDTH,
  preview: PREVIEW_PANEL_WIDTH,
}

export function SidePanel({
  session,
  plan,
  working,
  visible,
  view,
  previewUrl,
  onPreviewUrlChange,
}: {
  session: Session
  plan?: PlanSurface
  working: boolean
  visible: boolean
  view: SidePanelView
  previewUrl: string
  onPreviewUrlChange: (url: string) => void
}) {
  switch (view) {
    case 'diff':
      return <CodeDiffPanel session={session} visible={visible} />
    case 'preview':
      return <PreviewPanel url={previewUrl} onUrlChange={onPreviewUrlChange} />
    default:
      return <OverviewPanel session={session} plan={plan} working={working} />
  }
}
