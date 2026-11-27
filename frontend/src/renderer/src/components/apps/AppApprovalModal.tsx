import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { elevatedCapabilities, type PendingAppDraft } from '@/lib/api/apps'

// The one approval saving an app asks for: it wants AI credits or MCP tools.
export function AppApprovalModal({
  draft,
  saving,
  error,
  onApprove,
  onClose,
}: {
  draft: PendingAppDraft
  saving: boolean
  error?: string
  onApprove: () => void
  onClose: () => void
}) {
  const elevated = elevatedCapabilities(draft.capabilities)
  return (
    <Modal
      open
      onClose={onClose}
      size="md"
      title={`Allow “${draft.title}”?`}
      description="This app asks for more than a private app gets by default."
      footer={
        <>
          <div className="min-w-0 truncate text-[12px] text-danger">{error}</div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button variant="ghost" size="md" disabled={saving} onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" size="md" disabled={saving} onClick={onApprove}>
              {saving ? 'Saving…' : 'Allow and save'}
            </Button>
          </div>
        </>
      }
    >
      <ul className="space-y-3 text-[13px] text-ink">
        {elevated.includes('ai:complete') ? (
          <li>
            <p className="font-medium">Use AI</p>
            <p className="text-ink-2">Each use is billed to whoever has the app open.</p>
          </li>
        ) : null}
        {elevated.includes('tools:call') ? (
          <li>
            <p className="font-medium">Call tools</p>
            <p className="text-ink-2">
              {draft.allowed_tools.length
                ? 'It can call these tools as whoever has the app open:'
                : 'It declares tool access but names no tools, so it can call none.'}
            </p>
            {draft.allowed_tools.length ? (
              <p className="mt-1 font-mono text-[12px] text-ink-2">{draft.allowed_tools.join(', ')}</p>
            ) : null}
          </li>
        ) : null}
      </ul>
    </Modal>
  )
}
