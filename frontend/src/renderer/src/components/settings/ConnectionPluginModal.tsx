import { Loader2, Plug, ShieldCheck, Unplug, Wrench } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { IntegrationConnectionAccount, IntegrationPlugin } from '@/lib/api/types'
import {
  accountAddress,
  accountAlias,
  CAPABILITY_LABELS,
  pluginActionLabel,
  statusLabel,
} from './connectionFormatting'
import { PluginGlyph } from './ConnectionPluginVisuals'

export function ConnectionPluginModal({
  plugin,
  open,
  connecting,
  disconnectingAccountID,
  onClose,
  onConnect,
  onDisconnect,
}: {
  plugin?: IntegrationPlugin
  open: boolean
  connecting: boolean
  disconnectingAccountID?: string
  onClose: () => void
  onConnect: () => void
  onDisconnect: (account: IntegrationConnectionAccount) => void
}) {
  if (!plugin) return null
  const accounts = plugin.connection?.accounts ?? []
  const available = plugin.implementation.status === 'available'
  const toolCount = plugin.tools?.length ?? 0

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={plugin.name}
      description={plugin.description}
      icon={<PluginGlyph plugin={plugin} size={18} />}
      size="lg"
      footer={
        <>
          <span className="text-[12px] text-ink-3">
            {accounts.length > 0
              ? `${accounts.length} connected ${accounts.length === 1 ? 'account' : 'accounts'}`
              : 'No connected accounts'}
          </span>
          <Button variant="primary" disabled={!available || connecting} onClick={onConnect}>
            {connecting ? <Loader2 size={14} className="animate-spin" /> : <Plug size={14} />}
            {pluginActionLabel(plugin, connecting)}
          </Button>
        </>
      }
    >
      <div className="space-y-6">
        <ModalSection title={`Accounts ${accounts.length}`}>
          {accounts.length === 0 ? (
            <p className="text-[13px] text-ink-3">No accounts connected.</p>
          ) : (
            <div className="divide-y divide-border">
              {accounts.map((account) => (
                <ConnectedAccountListItem
                  key={account.id}
                  account={account}
                  disconnecting={disconnectingAccountID === account.id}
                  onDisconnect={() => onDisconnect(account)}
                />
              ))}
            </div>
          )}
        </ModalSection>

        <ModalSection title={`Tools ${toolCount}`}>
          {toolCount === 0 ? (
            <p className="text-[13px] text-ink-3">No tools implemented yet.</p>
          ) : (
            <div className="divide-y divide-border">
              {plugin.tools?.map((tool) => (
                <div key={tool.name} className="flex gap-3 py-2.5">
                  <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-control bg-surface-2 text-ink-2">
                    <Wrench size={14} />
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-[13px] font-medium text-ink" title={tool.name}>
                      {tool.name}
                    </p>
                    <p className="mt-0.5 text-[12px] leading-5 text-ink-3">{tool.description}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </ModalSection>

        <ModalSection title="Information">
          <div className="grid gap-y-2 text-[13px] sm:grid-cols-[160px_minmax(0,1fr)]">
            <InfoRow label="Provider" value={plugin.provider.name} />
            <InfoRow label="Category" value={statusLabel(plugin.category ?? 'app')} />
            <InfoRow
              label="Capabilities"
              value={plugin.capabilities.map((capability) => CAPABILITY_LABELS[capability]).join(', ')}
            />
            <InfoRow label="Implementation" value={statusLabel(plugin.implementation.status)} />
            {plugin.remote_mcp ? (
              <InfoRow label="Official MCP" value={statusLabel(plugin.remote_mcp.status)} />
            ) : null}
          </div>
        </ModalSection>

        {plugin.connection_notes?.length ? (
          <ModalSection title="Notes">
            <div className="space-y-1.5">
              {plugin.connection_notes.map((note) => (
                <div key={note} className="flex gap-2 text-[12px] leading-5 text-ink-3">
                  <ShieldCheck size={14} className="mt-0.5 shrink-0 text-ink-3" />
                  <span>{note}</span>
                </div>
              ))}
            </div>
          </ModalSection>
        ) : null}
      </div>
    </Modal>
  )
}

function ModalSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-t border-border pt-4 first:border-t-0 first:pt-0">
      <h3 className="mb-2 text-[13px] font-semibold text-ink">{title}</h3>
      {children}
    </section>
  )
}

function ConnectedAccountListItem({
  account,
  disconnecting,
  onDisconnect,
}: {
  account: IntegrationConnectionAccount
  disconnecting: boolean
  onDisconnect: () => void
}) {
  const address = accountAddress(account)
  const alias = accountAlias(account)
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 py-2.5">
      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-ink" title={address || account.id}>
          {address || account.id}
        </p>
        {alias ? (
          <p className="mt-0.5 truncate text-[12px] text-ink-3" title={alias}>
            {alias}
          </p>
        ) : null}
      </div>
      <Button
        variant="danger"
        size="sm"
        disabled={disconnecting}
        onClick={onDisconnect}
        className="min-h-10"
      >
        {disconnecting ? <Loader2 size={13} className="animate-spin" /> : <Unplug size={13} />}
        {disconnecting ? 'Disconnecting' : 'Disconnect'}
      </Button>
    </div>
  )
}

function InfoRow({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <>
      <span className="text-ink-3">{label}</span>
      <span className="min-w-0 text-ink">{value}</span>
    </>
  )
}
