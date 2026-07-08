import type { IntegrationPlugin } from '@/lib/api/types'

const pluginAssetUrls: Record<string, string> = {
  gmail: '/integrations/gmail.svg',
  google_calendar: '/integrations/google_calendar.svg',
  ink: '/integrations/ink.png',
  jaz: '/integrations/jaz.png',
  slack: '/integrations/slack.svg',
  telegram: '/integrations/telegram.svg',
  whatsapp: '/integrations/whatsapp.svg',
}

export function PluginIcon({ plugin, compact = false }: { plugin: IntegrationPlugin; compact?: boolean }) {
  const sizeClass = compact ? 'size-8' : 'size-9'
  const glyphSizeClass = compact ? 'size-5' : 'size-6'
  const iconSize = compact ? 16 : 18
  const assetUrl = pluginAssetUrl(plugin)

  if (assetUrl) {
    return (
      <span className={`grid ${sizeClass} shrink-0 place-items-center rounded-[8px] bg-bg ring-1 ring-border/70`}>
        <img src={assetUrl} alt="" className={`${glyphSizeClass} object-contain`} />
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
