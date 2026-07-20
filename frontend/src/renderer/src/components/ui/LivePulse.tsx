// "Working" indicator mark: a tiny ECG trace. A bright segment races along
// the flatline, jumps the spike (the beat), exits, and repeats — a literal
// live pulse in one inherited color. Pure CSS dash animation; reduced
// motion leaves the faint static trace.

export function LivePulse({ className }: { className?: string }) {
  const trace = 'M0 6 H5 L7 2 L9 10 L11 6 H16'
  return (
    <svg className={`live-pulse ${className ?? ''}`} viewBox="0 0 16 12" aria-hidden>
      <path className="live-pulse-trace" d={trace} />
      <path className="live-pulse-sweep" d={trace} pathLength={28} />
    </svg>
  )
}
