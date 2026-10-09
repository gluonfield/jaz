import type { MCPEntrypoint } from '@/lib/api/types'

// An app's icon is drawn as a mask in the current text colour, so it dims,
// brightens and themes like the built-in glyphs; its title's initial stands
// in when the server publishes none.
export function AppIcon({ app, size = 18 }: { app: Pick<MCPEntrypoint, 'icon' | 'title'>; size?: number }) {
  if (!app.icon) {
    return (
      <span aria-hidden className="shrink-0 font-semibold leading-none" style={{ fontSize: Math.round(size * 0.72) }}>
        {app.title.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return (
    <span
      aria-hidden
      className="shrink-0 bg-current"
      style={{ width: size, height: size, maskImage: `url("${app.icon}")`, maskSize: 'contain', maskRepeat: 'no-repeat', maskPosition: 'center' }}
    />
  )
}
