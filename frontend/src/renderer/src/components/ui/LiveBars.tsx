// Subtle "working" indicator: four tiny equalizer bars breathing at desynced
// tempos (own peak height, duration, and phase), so the skyline keeps
// evolving without ever rotating or flashing. Pure CSS in one inherited
// color; reduced motion renders a static skyline.

export function LiveBars({ className }: { className?: string }) {
  return (
    <span className={`live-bars ${className ?? ''}`} aria-hidden>
      <span />
      <span />
      <span />
      <span />
    </span>
  )
}
