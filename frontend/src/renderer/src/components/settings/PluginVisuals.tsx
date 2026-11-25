import github from '@/assets/integrations/github.svg'
import gmail from '@/assets/integrations/gmail.svg'
import google_calendar from '@/assets/integrations/google_calendar.svg'
import google_drive from '@/assets/integrations/google_drive.png'
import slack from '@/assets/integrations/slack.svg'
import telegram from '@/assets/integrations/telegram.svg'
import whatsapp from '@/assets/integrations/whatsapp.svg'
import type { Plugin } from '@/lib/api/plugins'

const pluginAssetUrls: Record<string, string> = {
  github,
  gmail,
  google_calendar,
  google_drive,
  slack,
  telegram,
  whatsapp,
}

export function PluginAvatar({ plugin, size = 'md' }: { plugin: Plugin; size?: 'sm' | 'md' | 'lg' }) {
  const sizeClass = size === 'sm' ? 'size-6 rounded-[6px]' : size === 'lg' ? 'size-10 rounded-[10px]' : 'size-7 rounded-[7px]'
  const src = pluginImageSrc(plugin)

  if (src) {
    return <img src={src} alt="" className={`${sizeClass} shrink-0 object-contain`} />
  }

  const initials = (plugin.icon.value || plugin.display_name.slice(0, 2)).toUpperCase()
  const textSize = size === 'sm' ? 'text-[10px]' : size === 'lg' ? 'text-[14px]' : 'text-[11px]'
  return (
    <span
      className={`grid ${sizeClass} shrink-0 place-items-center bg-surface-2 ${textSize} font-semibold text-ink`}
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
