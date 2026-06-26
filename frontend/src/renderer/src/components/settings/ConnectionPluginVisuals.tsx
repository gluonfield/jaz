import { Mail } from 'lucide-react'
import type { IntegrationPlugin } from '@/lib/api/types'

export function PluginIcon({ plugin, compact = false }: { plugin: IntegrationPlugin; compact?: boolean }) {
  const sizeClass = compact ? 'size-8' : 'size-9'
  const iconSize = compact ? 16 : 18

  if (plugin.icon.kind === 'url') {
    return (
      <img
        src={plugin.icon.value}
        alt=""
        className={`${sizeClass} shrink-0 rounded-control bg-surface-2 object-cover`}
      />
    )
  }

  if (plugin.icon.kind === 'asset' && plugin.icon.value === 'gmail') {
    return (
      <span className={`grid ${sizeClass} shrink-0 place-items-center rounded-control bg-surface-2 text-[#d93025]`}>
        <PluginGlyph plugin={plugin} size={iconSize} />
      </span>
    )
  }

  return (
    <span
      className={`grid ${sizeClass} shrink-0 place-items-center rounded-control bg-surface-2 text-[12px] font-medium text-ink`}
      style={plugin.icon.background ? { background: plugin.icon.background } : undefined}
    >
      <PluginGlyph plugin={plugin} size={iconSize} />
    </span>
  )
}

export function PluginGlyph({ plugin, size }: { plugin: IntegrationPlugin; size: number }) {
  if (plugin.icon.kind === 'asset' && plugin.icon.value === 'gmail') {
    return <Mail size={size} />
  }
  if (plugin.icon.kind === 'url') {
    return <img src={plugin.icon.value} alt="" className="size-4 rounded-[4px] object-cover" />
  }
  return <span>{plugin.icon.value || plugin.name.slice(0, 2).toUpperCase()}</span>
}
