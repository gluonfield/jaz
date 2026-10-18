import { CircleCheck, Users, type LucideIcon } from 'lucide-react'
import gmail from '@/assets/integrations/gmail.svg'
import google_calendar from '@/assets/integrations/google_calendar.svg'
import google_drive from '@/assets/integrations/google_drive.png'
import granola from '@/assets/integrations/granola.png'
import ink from '@/assets/integrations/ink.png'
import slack from '@/assets/integrations/slack.svg'
import telegram from '@/assets/integrations/telegram.svg'
import whatsapp from '@/assets/integrations/whatsapp.svg'
import type { IntegrationPlugin } from '@/lib/api/types'

const pluginGlyphs: Record<string, LucideIcon> = { tasks: CircleCheck, crm: Users }

const pluginAssetUrls: Record<string, string> = {
  gmail,
  google_calendar,
  google_drive,
  granola,
  ink,
  slack,
  telegram,
  whatsapp,
}

export function PluginIcon({ plugin, compact = false }: { plugin: IntegrationPlugin; compact?: boolean }) {
  const sizeClass = compact ? 'size-8' : 'size-9'
  const iconSize = compact ? 16 : 18
  const assetUrl = pluginAssetUrl(plugin)

  if (assetUrl || (plugin.icon.kind === 'asset' && pluginGlyphs[plugin.icon.value])) {
    return (
      <span className={`grid ${sizeClass} shrink-0 place-items-center rounded-[8px] bg-bg ring-1 ring-border/70`}>
        <PluginGlyph plugin={plugin} size={compact ? 20 : 24} />
      </span>
    )
  }

  if (plugin.icon.kind === 'url') {
    return (
      <img
        src={plugin.icon.value}
        alt=""
        className={`${sizeClass} shrink-0 rounded-[8px] bg-bg object-contain p-1 ring-1 ring-border/70`}
      />
    )
  }

  return (
    <span
      className={`grid ${sizeClass} shrink-0 place-items-center rounded-full bg-bg text-[12px] font-medium text-ink ring-1 ring-border/70`}
      style={plugin.icon.background ? { background: plugin.icon.background } : undefined}
    >
      <PluginGlyph plugin={plugin} size={iconSize} />
    </span>
  )
}

export function PluginGlyph({ plugin, size }: { plugin: IntegrationPlugin; size: number }) {
  const Glyph = plugin.icon.kind === 'asset' ? pluginGlyphs[plugin.icon.value] : undefined
  if (Glyph) {
    return <Glyph size={size} strokeWidth={1.75} aria-hidden />
  }
  const assetUrl = pluginAssetUrl(plugin)
  if (assetUrl) {
    return <img src={assetUrl} alt="" className="object-contain" style={{ width: size, height: size }} />
  }
  if (plugin.icon.kind === 'url') {
    return <img src={plugin.icon.value} alt="" className="size-4 rounded-[4px] object-contain" />
  }
  return <span>{plugin.icon.value || plugin.name.slice(0, 2).toUpperCase()}</span>
}

function pluginAssetUrl(plugin: IntegrationPlugin) {
  if (plugin.icon.kind !== 'asset') {
    return undefined
  }
  return pluginAssetUrls[plugin.icon.value]
}
