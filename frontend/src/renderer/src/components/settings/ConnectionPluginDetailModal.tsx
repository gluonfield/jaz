import { ArrowUp, Loader2, Plug, Plus, QrCode } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type { IntegrationPlugin, IntegrationTool } from '@/lib/api/types'
import {
  categoryLabel,
  pluginActionLabel,
  pluginCanConnect,
  titleCase,
} from './connectionFormatting'
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
      <div className="space-y-9 px-8 pb-9">
        {plugin.examples?.length ? <ExamplesBand plugin={plugin} /> : null}
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
  return (
    <div className="px-8 pb-8 pt-9">
      <PluginGlyph plugin={plugin} size={44} />
      <div className="mt-5 flex items-center justify-between gap-5">
        <div className="min-w-0">
          <h2 className="truncate text-2xl font-semibold leading-tight text-ink">{plugin.name}</h2>
          {plugin.description ? (
            <p className="mt-1.5 max-w-[58ch] text-[14px] leading-6 text-ink-2">
              {plugin.description}
            </p>
          ) : null}
        </div>
        <ConnectButton plugin={plugin} connecting={connecting} onConnect={onConnect} />
      </div>
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

function ExamplesBand({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <div
      className="rounded-card px-6 py-8"
      style={{
        background:
          'radial-gradient(135% 135% at 12% 15%, oklch(0.52 0.1 262) 0%, transparent 55%),' +
          'radial-gradient(120% 130% at 88% 92%, oklch(0.46 0.09 292) 0%, transparent 55%),' +
          'linear-gradient(135deg, oklch(0.33 0.06 266) 0%, oklch(0.29 0.05 282) 100%)',
      }}
    >
      <div className="mx-auto flex max-w-[480px] flex-col gap-2.5">
        {plugin.examples?.map((example) => (
          <div
            key={example}
            className="flex items-center gap-3 rounded-full bg-black/40 px-4 py-2.5 ring-1 ring-white/10"
          >
            <PluginGlyph plugin={plugin} size={17} />
            <span className="shrink-0 text-[13px] font-medium text-white">{plugin.name}</span>
            <span className="min-w-0 flex-1 truncate text-[13px] text-white/70">{example}</span>
            <span className="grid size-7 shrink-0 place-items-center rounded-full bg-white/10 text-white/80">
              <ArrowUp size={14} />
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

function ToolsSection({ tools }: { tools: IntegrationTool[] }) {
  return (
    <section>
      <SectionLabel count={tools.length}>Tools</SectionLabel>
      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        {tools.map((tool) => (
          <div
            key={tool.name}
            className="rounded-card bg-surface px-3 py-2 ring-1 ring-border/60"
            title={tool.name}
          >
            <p className="truncate text-[12px] font-medium text-ink">{formatToolName(tool.name)}</p>
            <p className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-ink-2">
              {tool.description}
            </p>
          </div>
        ))}
      </div>
    </section>
  )
}

function formatToolName(name: string): string {
  const spaced = name.replace(/_/g, ' ')
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
      return 'Agent tools'
    case 'browser':
      return 'Browser'
    default:
      return titleCase(value)
  }
}

function developerLabel(plugin: IntegrationPlugin): string {
  if (plugin.implementation.owner === 'jaz') return 'Jaz'
  return titleCase(plugin.implementation.owner)
}

function authDescription(kind?: string): string {
  switch (kind) {
    case 'oauth':
      return 'Browser sign-in'
    case 'session':
      return 'QR sign-in'
    case 'remote_mcp':
      return 'Remote MCP'
    case 'mcp_connection':
      return 'Browser sign-in'
    case 'browser_local':
      return 'Local browser'
    default:
      return 'Configured by Jaz'
  }
}
