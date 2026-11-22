import { Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { IconButton } from '@/components/ui/IconButton'
import { Modal } from '@/components/ui/Modal'
import type { PluginSource } from '@/lib/api/plugins'

export function MarketplacesModal({
  sources,
  removingID,
  onClose,
  onAdd,
  onRemove,
}: {
  sources: PluginSource[]
  removingID?: string
  onClose: () => void
  onAdd: () => void
  onRemove: (id: string) => void
}) {
  return (
    <Modal
      open
      onClose={onClose}
      title="Git marketplaces"
      description="Plugin catalogs synced from Git repositories."
    >
      <div className="space-y-3">
        {sources.length === 0 ? (
          <p className="rounded-card bg-surface px-4 py-6 text-center text-[13px] text-ink-3">
            No Git marketplaces synced yet.
          </p>
        ) : (
          <div className="divide-y divide-border/50 rounded-card bg-surface">
            {sources.map((source) => (
              <div key={source.id} className="flex items-center justify-between gap-3 px-3.5 py-2.5">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="truncate text-[13px] font-medium text-ink">
                      {source.repository}
                      {source.ref ? <span className="ml-1 font-normal text-ink-3">@{source.ref}</span> : null}
                    </p>
                    {source.built_in ? (
                      <span className="rounded-full bg-surface-2 px-1.5 py-0.5 text-[10px] font-medium text-ink-3">
                        Official
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-0.5 truncate text-[12px] text-ink-3">
                    {source.commit_sha ? source.commit_sha.slice(0, 8) : 'Not synced yet'}
                    {source.auto_update ? ' · Auto-update' : ''}
                  </p>
                </div>
                {!source.built_in ? (
                  <IconButton
                    variant="ghost"
                    size="xs"
                    aria-label={`Remove ${source.repository}`}
                    title={`Remove ${source.repository}`}
                    disabled={removingID === source.id}
                    onClick={() => onRemove(source.id)}
                  >
                    <Trash2 size={14} />
                  </IconButton>
                ) : null}
              </div>
            ))}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" onClick={onClose}>
            Done
          </Button>
          <Button variant="primary" onClick={onAdd}>
            <Plus size={14} />
            Add marketplace
          </Button>
        </div>
      </div>
    </Modal>
  )
}
