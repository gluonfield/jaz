import { ArrowUp, Loader2, Plug, Plus, QrCode } from 'lucide-react'
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
    <Modal open onClose={onClose} title={plugin.name} size="wide" chromeless>
      <Hero plugin={plugin} connecting={connecting} onConnect={() => onConnect(plugin)} />
      <div className="space-y-8 px-7 py-7">
        {plugin.examples?.length ? <ExamplesSection plugin={plugin} /> : null}
        {plugin.tools?.length ? <ToolsSection tools={plugin.tools} /> : null}
        <InformationSection plugin={plugin} />
      </div>
    </Modal>
  )
}

function Hero({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onConnect: () => void
}) {
  const connected = plugin.connection?.status === 'connected'

  return (
    <div className="border-b border-border bg-surface px-7 pb-7 pt-8">
      <div className="flex items-center gap-4">
        <span className="grid size-14 shrink-0 place-items-center rounded-card bg-bg ring-1 ring-border">
          <PluginGlyph plugin={plugin} size={30} />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[17px] font-semibold leading-tight text-ink">{plugin.name}</h2>
          <div className="mt-1.5 flex items-center gap-2 text-[13px] text-ink-2">
            <span>{categoryLabel(plugin.category)}</span>
            <span className="text-ink-3" aria-hidden="true">
              ·
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className={`size-1.5 rounded-full ${connected ? 'bg-ok' : 'bg-ink-3'}`} />
              {connected ? 'Connected' : 'Not connected'}
            </span>
          </div>
        </div>
        <ConnectButton plugin={plugin} connecting={connecting} onConnect={onConnect} />
      </div>
      {plugin.description ? (
        <p className="mt-4 max-w-[58ch] text-[13px] leading-6 text-ink-2">{plugin.description}</p>
      ) : null}
    </div>
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
    <Button
      variant="primary"
      size="lg"
      disabled={!available || connecting}
      onClick={onConnect}
      className="shrink-0"
    >
      <Icon size={14} className={connecting ? 'animate-spin' : undefined} />
      {pluginActionLabel(plugin, connecting)}
    </Button>
  )
}

function ExamplesSection({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <section>
      <SectionLabel>Try asking</SectionLabel>
      <div className="space-y-2">
        {plugin.examples?.map((example) => (
          <div
            key={example}
            className="flex items-center gap-3 rounded-full bg-surface px-4 py-2.5"
          >
            <PluginGlyph plugin={plugin} size={17} />
            <span className="shrink-0 text-[13px] font-medium text-ink">{plugin.name}</span>
            <span className="min-w-0 flex-1 truncate text-[13px] text-ink-2">{example}</span>
            <span className="grid size-7 shrink-0 place-items-center rounded-full bg-surface-2 text-ink-2">
              <ArrowUp size={14} />
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

function ToolsSection({ tools }: { tools: IntegrationTool[] }) {
  return (
    <section>
      <SectionLabel count={tools.length}>Tools</SectionLabel>
      <div className="flex flex-wrap gap-2">
        {tools.map((tool) => (
          <span
            key={tool.name}
            className="rounded-full px-3 py-1.5 text-[12px] text-ink-2 ring-1 ring-border"
            title={tool.name}
          >
            {formatToolName(tool.name)}
          </span>
        ))}
      </div>
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
      <SectionLabel>Information</SectionLabel>
      <dl className="divide-y divide-border/60 text-[13px]">
        {rows.map(([label, value]) => (
          <div key={label} className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-5 py-2.5">
            <dt className="text-ink-3">{label}</dt>
            <dd className="min-w-0 text-ink">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

function SectionLabel({ children, count }: { children: string; count?: number }) {
  return (
    <p className="mb-3 flex items-center gap-2 text-[13px] font-medium text-ink">
      {children}
      {count !== undefined ? <span className="tabular-nums text-ink-3">{count}</span> : null}
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
