import { ArrowRight, Loader2, Plug, Plus, QrCode } from 'lucide-react'
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
        {plugin.examples?.length ? <ExamplesBand plugin={plugin} /> : null}
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

function ExamplesBand({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <div
      className="space-y-2 rounded-card px-4 py-5"
      style={{
        background:
          'linear-gradient(120deg, color-mix(in oklab, var(--color-primary) 28%, var(--color-bg)) 0%, color-mix(in oklab, var(--color-primary) 10%, var(--color-surface)) 55%, var(--color-surface) 100%)',
      }}
    >
      {plugin.examples?.map((example) => (
        <div
          key={example}
          className="flex items-center gap-2.5 rounded-full bg-bg/80 px-3.5 py-2.5"
        >
          <PluginGlyph plugin={plugin} size={16} />
          <span className="shrink-0 text-[13px] font-medium text-ink">{plugin.name}</span>
          <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2">{example}</span>
          <span className="grid size-6 shrink-0 place-items-center rounded-full bg-surface-2 text-ink-3">
            <ArrowRight size={13} />
          </span>
        </div>
      ))}
    </div>
  )
}

function ToolsSection({ tools }: { tools: IntegrationTool[] }) {
  return (
    <section>
      <SectionHeading label="Tools" count={tools.length} />
      <ul className="grid grid-cols-1 gap-x-8 gap-y-1.5 sm:grid-cols-2">
        {tools.map((tool) => (
          <li key={tool.name} className="truncate text-[13px] text-ink" title={tool.name}>
            {formatToolName(tool.name)}
          </li>
        ))}
      </ul>
    </section>
  )
}

function formatToolName(name: string): string {
  const spaced = name.replace(/_/g, ' ').trim()
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
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
