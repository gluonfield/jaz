import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { AppWindow, FileText, LoaderCircle } from 'lucide-react'
import { memo, use, useMemo, useState, type ReactNode } from 'react'
import { AppApprovalModal } from '@/components/apps/AppApprovalModal'
import { DesktopAppButton, useDesktopApps } from '@/components/apps/useDesktopApps'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { Button } from '@/components/ui/Button'
import { elevatedCapabilities, saveAppDraft, type BundleKind, type PendingAppDraft } from '@/lib/api/apps'
import type { ACPToolCall } from '@/lib/api/types'
import { ArtifactPanelContext } from '@/lib/artifactPanel'
import { installFolder } from '@/lib/desktopApps'
import { keys } from '@/lib/query/keys'
import type { PublishedAppRef } from '@/lib/sidePanelTabs'
import { isAppDraftExpired, parseAppToolOutput } from './appTool'

function Card({ icon, title, note, noteTitle, children }: { icon: ReactNode; title: string; note: string; noteTitle?: string; children?: ReactNode }) {
  return (
    <div className="flex w-full max-w-[var(--prose-max)] flex-wrap items-center gap-x-3 gap-y-2 rounded-card border border-border bg-surface px-3.5 py-3">
      <span className="shrink-0 text-ink-3">{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-medium text-ink">{title}</p>
        <p className="truncate text-[12px] text-ink-3" title={noteTitle}>{note}</p>
      </div>
      {children ? <div className="flex shrink-0 items-center gap-1.5">{children}</div> : null}
    </div>
  )
}

const kindIcon = (kind: BundleKind) =>
  kind === 'document' ? <FileText size={16} aria-hidden /> : <AppWindow size={16} aria-hidden />

function SavedApp({ app }: { app: PublishedAppRef & { kind: BundleKind } }) {
  const panel = use(ArtifactPanelContext)
  const desktop = useDesktopApps()
  const launcher = desktop.records.get(app.id)?.launcherPath
  return (
    <Card
      icon={kindIcon(app.kind)}
      title={app.title}
      note={launcher ? `Installed in ${installFolder(launcher)}` : 'Saved to your Apps'}
      noteTitle={launcher}
    >
      <DesktopAppButton desktop={desktop} app={app.id} />
      {panel ? (
        <Button variant="primary" size="sm" onClick={() => panel.openPublishedApp(app)}>
          Show App
        </Button>
      ) : (
        <Link
          to="/apps/$appId"
          params={{ appId: app.id }}
          className="inline-flex h-7 shrink-0 items-center rounded-full bg-primary px-2.5 text-[12px] font-medium text-on-primary transition-colors hover:bg-primary-strong"
        >
          Show App
        </Link>
      )}
      {launcher ? <ActionMenu label="Installed app actions" actions={desktop.actions(app.id)} /> : null}
      {desktop.dialog}
    </Card>
  )
}

function PendingApp({ draft }: { draft: PendingAppDraft }) {
  const queryClient = useQueryClient()
  const [reviewing, setReviewing] = useState(false)
  const save = useMutation({
    mutationFn: () => saveAppDraft(draft),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      setReviewing(false)
    },
  })
  if (save.data) return <SavedApp app={save.data} />
  if (isAppDraftExpired(draft.expires_at)) {
    return <Card icon={kindIcon(draft.kind)} title={draft.title} note="Not saved. Ask for it again to save it." />
  }
  const needsApproval = elevatedCapabilities(draft.capabilities).length > 0
  const error = save.isError ? (save.error as Error).message : undefined
  return (
    <>
      <Card
        icon={kindIcon(draft.kind)}
        title={draft.title}
        note={(!reviewing && error) || (needsApproval ? 'Needs your approval before it is saved' : 'Ready to save')}
      >
        <Button
          variant="primary"
          size="sm"
          disabled={save.isPending}
          onClick={() => (needsApproval ? setReviewing(true) : save.mutate())}
        >
          {needsApproval ? 'Review permissions' : save.isPending ? 'Saving…' : 'Save app'}
        </Button>
      </Card>
      {reviewing ? (
        <AppApprovalModal
          draft={draft}
          saving={save.isPending}
          error={error}
          onApprove={() => save.mutate()}
          onClose={() => setReviewing(false)}
        />
      ) : null}
    </>
  )
}

// An app the agent saved, shown in the message stream with its two actions.
export const AppDeliverableCard = memo(function AppDeliverableCard({ call }: { call: ACPToolCall }) {
  const result = useMemo(() => parseAppToolOutput(call.raw_output), [call.raw_output])
  if (!result) {
    return <Card icon={<LoaderCircle size={16} className="animate-spin text-running" aria-hidden />} title="Saving app…" note="It will appear here in a moment." />
  }
  return 'app' in result ? <SavedApp app={result.app} /> : <PendingApp draft={result.draft} />
})
