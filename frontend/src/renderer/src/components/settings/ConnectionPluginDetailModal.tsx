import { Loader2, Plug, Plus, QrCode } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { IntegrationPlugin, IntegrationTool } from '@/lib/api/types'
import { pluginActionLabel, pluginCanConnect } from './connectionFormatting'
import { PluginGlyph } from './ConnectionPluginVisuals'

export function ConnectionPluginDetailModal({
  plugin,
  connecting,
  onClose,
  onConnect,
}: {
  plugin: IntegrationPlugin | null
  connecting: boolean
  onClose: () => void
  onConnect: (plugin: IntegrationPlugin) => void
}) {
  if (!plugin) return null

  return (
    <Modal
      open
      onClose={onClose}
      title={plugin.name}
      description={categoryLabel(plugin.category)}
      icon={<PluginGlyph plugin={plugin} size={18} />}
      headerAccessory={
        <ConnectButton plugin={plugin} connecting={connecting} onConnect={() => onConnect(plugin)} />
      }
      size="lg"
    >
      <div className="space-y-7">
        <Hero plugin={plugin} />
        {plugin.tools?.length ? <ToolsSection tools={plugin.tools} /> : null}
        <InformationSection plugin={plugin} />
      </div>
    </Modal>
  )
}

function ConnectButton({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onConnect: () => void
}) {
  const sessionAuth = plugin.auth[0]?.kind === 'session'
  const available = pluginCanConnect(plugin)
  const connected = plugin.connection?.status === 'connected'
  let Icon = sessionAuth ? QrCode : Plug
  if (available && connected && plugin.multi_account) Icon = Plus
  if (connecting) Icon = Loader2

  return (
    <Button variant="primary" size="lg" disabled={!available || connecting} onClick={onConnect}>
      <Icon size={14} className={connecting ? 'animate-spin' : undefined} />
      {pluginActionLabel(plugin, connecting)}
    </Button>
  )
}

function Hero({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <div className="rounded-card bg-[linear-gradient(135deg,var(--color-primary-soft),var(--color-surface-2))] px-6 py-9 text-center">
      <span className="mx-auto grid size-16 place-items-center rounded-[18px] bg-bg ring-1 ring-border/70">
        <PluginGlyph plugin={plugin} size={32} />
      </span>
      {plugin.description ? (
        <p className="mx-auto mt-5 max-w-sm text-[13px] leading-6 text-ink-2">{plugin.description}</p>
      ) : null}
    </div>
  )
}

function ToolsSection({ tools }: { tools: IntegrationTool[] }) {
  return (
    <section>
      <SectionHeading label="Tools" count={tools.length} />
      <ul className="max-h-[min(260px,38dvh)] divide-y divide-border/50 overflow-y-auto rounded-card bg-surface">
        {tools.map((tool) => (
          <li key={tool.name} className="px-3.5 py-3">
            <p className="font-mono text-[12px] text-ink">{tool.name}</p>
            {tool.description ? (
              <p className="mt-1 text-[12px] leading-5 text-ink-2">{tool.description}</p>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}

function InformationSection({ plugin }: { plugin: IntegrationPlugin }) {
  const rows: [string, string][] = [
    ['Capabilities', capabilityLabels(plugin)],
    ['Developer', developerLabel(plugin)],
    ['Category', categoryLabel(plugin.category)],
    ['Sign in', authDescription(plugin.auth[0]?.kind)],
  ]

  return (
    <section>
      <p className="mb-1 text-[13px] font-medium text-ink">Information</p>
      <dl className="divide-y divide-border/50 text-[13px]">
        {rows.map(([label, value]) => (
          <div key={label} className="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-5 py-3">
            <dt className="text-ink-3">{label}</dt>
            <dd className="min-w-0 text-ink">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

function SectionHeading({ label, count }: { label: string; count: number }) {
  return (
    <p className="mb-3 flex items-center gap-2 text-[13px] font-medium text-ink">
      {label}
      <span className="tabular-nums text-ink-3">{count}</span>
    </p>
  )
}

function capabilityLabels(plugin: IntegrationPlugin): string {
  if (plugin.capabilities.length === 0) return 'None'
  return plugin.capabilities.map(capabilityLabel).join(', ')
}

function capabilityLabel(value: string): string {
  switch (value) {
    case 'act':
      return 'Actions'
    case 'sync':
      return 'Sync'
    case 'materialize':
      return 'Memory'
    case 'mcp':
      return 'MCP tools'
    case 'browser':
      return 'Browser'
    default:
      return statusLabel(value)
  }
}

function developerLabel(plugin: IntegrationPlugin): string {
  if (plugin.implementation.owner === 'jaz') return 'Jaz'
  return statusLabel(plugin.implementation.owner)
}

function categoryLabel(value?: string): string {
  return value ? statusLabel(value) : 'Integration'
}

function authDescription(kind?: string): string {
  switch (kind) {
    case 'oauth':
      return 'Browser sign-in'
    case 'session':
      return 'QR sign-in'
    case 'remote_mcp':
      return 'Remote MCP'
    case 'browser_local':
      return 'Local browser'
    default:
      return 'Configured by Jaz'
  }
}

function statusLabel(value: string): string {
  return value
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
