// Subtle "working" indicator: three small square dots (the brand's dither
// language) with a slow opacity wave traveling across them. Pure CSS, one
// color inherited from the parent — quiet, no spinner, no confetti.

export function LiveDots({ className }: { className?: string }) {
  return (
    <span className={`live-dots ${className ?? ''}`} aria-hidden>
      <span />
      <span />
      <span />
    </span>
  )
}
