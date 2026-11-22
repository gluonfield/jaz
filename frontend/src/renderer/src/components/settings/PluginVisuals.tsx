import gmail from '@/assets/integrations/gmail.svg'
import google_calendar from '@/assets/integrations/google_calendar.svg'
import google_drive from '@/assets/integrations/google_drive.png'
import slack from '@/assets/integrations/slack.svg'
import telegram from '@/assets/integrations/telegram.svg'
import whatsapp from '@/assets/integrations/whatsapp.svg'
import type { Plugin } from '@/lib/api/plugins'

const pluginAssetUrls: Record<string, string> = {
  gmail,
  google_calendar,
  google_drive,
  slack,
  telegram,
  whatsapp,
}

export function PluginAvatar({ plugin, size = 'md' }: { plugin: Plugin; size?: 'sm' | 'md' | 'lg' }) {
  const boxClass = size === 'sm' ? 'size-8 rounded-[8px]' : size === 'lg' ? 'size-11 rounded-[11px]' : 'size-10 rounded-[10px]'
  const imgSize = size === 'sm' ? 20 : size === 'lg' ? 28 : 24
  const src = pluginImageSrc(plugin)

  if (src) {
    return (
      <span className={`grid ${boxClass} shrink-0 place-items-center bg-surface ring-1 ring-border/70`}>
        <img src={src} alt="" className="object-contain" style={{ width: imgSize, height: imgSize }} />
      </span>
    )
  }

  const initials = (plugin.icon.value || plugin.display_name.slice(0, 2)).toUpperCase()
  return (
    <span
      className={`grid ${boxClass} shrink-0 place-items-center bg-surface text-[12px] font-semibold text-ink ring-1 ring-border/70`}
      style={plugin.icon.background ? { background: plugin.icon.background } : undefined}
    >
      {initials}
    </span>
  )
}

export function PluginGlyph({ plugin, size }: { plugin: Plugin; size: number }) {
  const src = pluginImageSrc(plugin)
  if (src) {
    return <img src={src} alt="" className="object-contain" style={{ width: size, height: size }} />
  }
  const initials = (plugin.icon.value || plugin.display_name.slice(0, 2)).toUpperCase()
  return <span className="font-semibold text-ink">{initials}</span>
}

function pluginImageSrc(plugin: Plugin): string | undefined {
  if (plugin.icon.kind === 'asset') return pluginAssetUrls[plugin.icon.value]
  if (plugin.icon.kind === 'data' || plugin.icon.kind === 'url') return plugin.icon.value
  if (plugin.icon_data_url) return plugin.icon_data_url
  return undefined
}
