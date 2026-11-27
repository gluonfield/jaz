import { Link } from '@tanstack/react-router'
import { AlertTriangle, CheckCircle2, Clock, ExternalLink, LoaderCircle, Share2, ShieldCheck } from 'lucide-react'
import { memo, useMemo, useState } from 'react'
import { PublishAppModal } from '@/components/apps/PublishAppModal'
import { Button } from '@/components/ui/Button'
import { formatAppBytes, type JazApp } from '@/lib/api/apps'
import type { ACPToolCall } from '@/lib/api/types'
import { extractAppDraftToolError, isAppDraftExpired, parseAppDraftToolOutput } from './appDraftTool'
import { isRunningToolStatus } from './toolPresentation'

export const AppDraftCard = memo(function AppDraftCard({
  call,
  active = false,
}: {
  call: ACPToolCall
  active?: boolean
}) {
  const [modalOpen, setModalOpen] = useState(false)
  const [publishedApp, setPublishedApp] = useState<JazApp | null>(null)

  const draft = useMemo(() => parseAppDraftToolOutput(call.raw_output), [call.raw_output])
  const running = isRunningToolStatus(call.status) && (active || call.raw_output == null)

  if (running) {
    return (
      <div className="my-1 flex items-center gap-2 rounded-card border border-border bg-surface px-3.5 py-2.5 text-[12.5px] text-ink-2">
        <LoaderCircle size={14} className="shrink-0 animate-spin text-running" aria-hidden />
        <span>Staging Jaz App preflight draft…</span>
      </div>
    )
  }

  if (!draft) {
    const detail = extractAppDraftToolError(call.raw_output)
    return (
      <div className="my-1 flex items-start gap-2 rounded-card border border-border bg-danger-soft/50 px-3.5 py-2.5 text-[12.5px] text-danger">
        <AlertTriangle size={14} className="mt-0.5 shrink-0" aria-hidden />
        <span className="min-w-0 break-words">
          {detail ? `App draft staging failed: ${detail}` : 'App draft staging failed.'}
        </span>
      </div>
    )
  }

  const expired = !publishedApp && isAppDraftExpired(draft.expires_at)

  return (
    <div className="my-1.5 rounded-card border border-border bg-surface px-3.5 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {publishedApp ? (
            <CheckCircle2 size={15} className="shrink-0 text-emerald-500" aria-hidden />
          ) : expired ? (
            <Clock size={15} className="shrink-0 text-ink-3" aria-hidden />
          ) : (
            <ShieldCheck size={15} className="shrink-0 text-primary" aria-hidden />
          )}
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-[13px] font-semibold text-ink">{publishedApp?.title ?? draft.title}</span>
              <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-px text-[10px] font-medium text-ink-3">
                {draft.kind === 'document' ? 'Document' : 'Jaz App'}
              </span>
            </div>
            <p className="truncate font-mono text-[11px] text-ink-3">
              {draft.file_count} {draft.file_count === 1 ? 'file' : 'files'} · {formatAppBytes(draft.size_bytes)} ·{' '}
              {draft.bundle_sha256.slice(0, 12)}
              {draft.warnings.length > 0 ? ` · ${draft.warnings.length} warning${draft.warnings.length === 1 ? '' : 's'}` : ''}
            </p>
          </div>
        </div>

        {publishedApp ? (
          <Link
            to="/apps/$appId"
            params={{ appId: publishedApp.id }}
            className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-primary px-3 py-1.5 text-[12px] font-medium text-on-primary transition-colors hover:bg-primary-strong"
          >
            <ExternalLink size={12} aria-hidden />
            Open App
          </Link>
        ) : expired ? (
          <span className="shrink-0 rounded-full bg-surface-2 px-2.5 py-1 text-[11.5px] font-medium text-ink-3">
            Draft expired
          </span>
        ) : (
          <Button variant="primary" size="sm" onClick={() => setModalOpen(true)}>
            <Share2 size={12} aria-hidden />
            Review &amp; Publish
          </Button>
        )}
      </div>

      {modalOpen ? (
        <PublishAppModal
          open
          onClose={() => setModalOpen(false)}
          initialTitle={draft.title}
          initialDescription={draft.description}
          kind={draft.kind}
          initialDraft={draft}
          onPublished={(app) => setPublishedApp(app)}
        />
      ) : null}
    </div>
  )
})
