import type { IntegrationPlugin } from '@/lib/api/types'

export function PluginIcon({ plugin, compact = false }: { plugin: IntegrationPlugin; compact?: boolean }) {
  const sizeClass = compact ? 'size-8' : 'size-9'
  const iconSize = compact ? 16 : 18

  if (plugin.icon.kind === 'url') {
    return (
      <img
        src={plugin.icon.value}
        alt=""
        className={`${sizeClass} shrink-0 rounded-[8px] bg-bg object-cover ring-1 ring-border/70`}
      />
    )
  }

  if (plugin.icon.kind === 'asset' && plugin.icon.value === 'gmail') {
    return (
      <span className={`grid ${sizeClass} shrink-0 place-items-center rounded-full bg-bg text-[#d93025] ring-1 ring-border/70`}>
        <PluginGlyph plugin={plugin} size={iconSize} />
      </span>
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
  if (plugin.icon.kind === 'asset' && plugin.icon.value === 'gmail') {
    return <GmailLogo size={size} />
  }
  if (plugin.icon.kind === 'url') {
    return <img src={plugin.icon.value} alt="" className="size-4 rounded-[4px] object-cover" />
  }
  return <span>{plugin.icon.value || plugin.name.slice(0, 2).toUpperCase()}</span>
}

function GmailLogo({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#4285F4" d="M20 18h2V7.6l-2 1.5V18z" />
      <path fill="#34A853" d="M2 7.6V18h2V9.1L2 7.6z" />
      <path fill="#FBBC04" d="M20 6v3.1l2-1.5V5.2c0-.9-1-1.4-1.7-.9L20 4.5V6z" />
      <path fill="#EA4335" d="M4 9.1V6l8 6 8-6V4.5l-8 6-8-6v4.6z" />
      <path fill="#C5221F" d="M2 5.2v2.4l2 1.5V4.5l-.3-.2C3 .8 2 .3 2 1.2z" />
    </svg>
  )
}
