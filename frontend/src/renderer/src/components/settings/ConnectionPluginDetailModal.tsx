import { ArrowRight, Loader2, Plug, Plus, Sparkles, Wrench } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import type {
  IntegrationPlugin,
  IntegrationSkill,
  IntegrationTool,
} from '@/lib/api/types'
import { adapterRequiredDescription, pluginActionLabel } from './connectionFormatting'
import { PluginGlyph, PluginIcon } from './ConnectionPluginVisuals'

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
      description={subtitle(plugin)}
      icon={<PluginGlyph plugin={plugin} size={18} />}
      size="lg"
    >
      <div className="space-y-7">
        <PreviewBand plugin={plugin} />
        <p className="max-w-2xl text-[13px] leading-6 text-ink-2">{plugin.description}</p>
        <AppsSection
          plugin={plugin}
          connecting={connecting}
          onConnect={() => onConnect(plugin)}
        />
        {plugin.tools?.length ? <ToolsSection tools={plugin.tools} /> : null}
        {plugin.skills?.length ? <SkillsSection skills={plugin.skills} /> : null}
        <InformationSection plugin={plugin} />
      </div>
    </Modal>
  )
}

function PreviewBand({ plugin }: { plugin: IntegrationPlugin }) {
  return (
    <div className="rounded-card bg-[linear-gradient(135deg,var(--color-primary-soft),var(--color-surface-2))] px-5 py-7">
      <div className="mx-auto flex w-fit max-w-full items-center gap-2 rounded-full bg-bg/90 px-4 py-2.5 shadow-raised">
        <PluginGlyph plugin={plugin} size={16} />
        <span className="shrink-0 text-[13px] font-medium text-ink">{plugin.name}</span>
        <span className="min-w-0 truncate text-[13px] text-ink-2">{previewText(plugin)}</span>
        <span className="grid size-7 shrink-0 place-items-center rounded-full bg-surface-2 text-ink-2">
          <ArrowRight size={14} />
        </span>
      </div>
    </div>
  )
}

function AppsSection({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onConnect: () => void
}) {
  const available = plugin.implementation.status === 'available'
  const connected = plugin.connection?.status === 'connected'
  const adapterRequired = plugin.implementation.status === 'adapter_required'
  const ConnectIcon = connecting ? Loader2 : adapterRequired ? Wrench : available && connected && plugin.multi_account ? Plus : Plug

  return (
    <section>
      <SectionHeading label="Apps" count={1} />
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 rounded-card px-0 py-2">
        <div className="flex min-w-0 items-center gap-3">
          <PluginIcon plugin={plugin} />
          <div className="min-w-0">
            <p className="truncate text-[13px] font-medium text-ink">{plugin.name}</p>
            <p className="mt-0.5 line-clamp-2 text-[13px] leading-5 text-ink-2">
              {appDescription(plugin)}
            </p>
            {adapterRequired ? (
              <p className="mt-1 text-[12px] leading-5 text-ink-3">
                {adapterRequiredDescription(plugin)}
              </p>
            ) : null}
          </div>
        </div>
        <Button variant="secondary" size="md" disabled={!available || connecting} onClick={onConnect}>
          <ConnectIcon size={14} className={connecting ? 'animate-spin' : undefined} />
          {pluginActionLabel(plugin, connecting)}
        </Button>
      </div>
    </section>
  )
}

function ToolsSection({ tools }: { tools: IntegrationTool[] }) {
  return (
    <section>
      <SectionHeading label="Tools" count={tools.length} />
      <div className="space-y-4">
        {tools.map((tool) => (
          <div key={tool.name} className="flex min-w-0 items-start gap-3">
            <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-control bg-surface text-ink-2">
              <Wrench size={15} />
            </span>
            <div className="min-w-0">
              <p className="truncate text-[13px] font-medium text-ink">{toolTitle(tool.name)}</p>
              <p className="mt-0.5 line-clamp-2 text-[13px] leading-5 text-ink-2">
                {tool.description}
              </p>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function SkillsSection({ skills }: { skills: IntegrationSkill[] }) {
  return (
    <section>
      <SectionHeading label="Skills" count={skills.length} />
      <div className="space-y-4">
        {skills.map((skill) => (
          <div key={skill.id} className="flex min-w-0 items-start gap-3">
            <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-control bg-surface text-primary">
              <Sparkles size={15} />
            </span>
            <div className="min-w-0">
              <p className="truncate text-[13px] font-medium text-ink">{skill.name}</p>
              {skill.description ? (
                <p className="mt-0.5 line-clamp-2 text-[13px] leading-5 text-ink-2">
                  {skill.description}
                </p>
              ) : null}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function InformationSection({ plugin }: { plugin: IntegrationPlugin }) {
  const rows = [
    ['Capabilities', capabilityLabels(plugin)],
    ['Developer', developerLabel(plugin)],
    ['Category', categoryLabel(plugin.category)],
    ['Sign in', authDescription(plugin.auth[0]?.kind)],
  ]

  return (
    <section>
      <p className="mb-4 text-[13px] font-medium text-ink">Information</p>
      <dl className="grid grid-cols-[128px_minmax(0,1fr)] gap-x-7 gap-y-4 text-[13px]">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
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
    <p className="mb-4 flex items-center gap-2 text-[13px] font-medium text-ink">
      {label}
      <span className="tabular-nums text-ink-3">{count}</span>
    </p>
  )
}

function subtitle(plugin: IntegrationPlugin): string {
  if (plugin.id === 'gmail') return 'Search threads and send drafts'
  if (plugin.category === 'chat') return `Read and manage ${plugin.name}`
  return plugin.provider.name
}

function previewText(plugin: IntegrationPlugin): string {
  if (plugin.id === 'gmail') return 'Search threads, draft replies, or send approved drafts'
  if (plugin.category === 'chat') return 'Search conversations, sync memory, or send messages'
  return plugin.description || `Use ${plugin.name} from Jaz`
}

function appDescription(plugin: IntegrationPlugin): string {
  if (plugin.id === 'gmail') return 'Search threads, draft replies, and send approved drafts'
  if (plugin.id === 'whatsapp') return 'Sync chats and send WhatsApp messages'
  if (plugin.id === 'telegram') return 'Sync chats and send Telegram messages'
  return plugin.description || `Use ${plugin.name} from Jaz`
}

function toolTitle(name: string): string {
  return name
    .replace(/^[^_]+_/, '')
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
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
      return 'QR pairing'
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
