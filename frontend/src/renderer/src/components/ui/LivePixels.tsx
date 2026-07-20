// Tiny square-pixel "working" indicator: a comet of pixels chases around the
// perimeter of a 5x5 grid while inner pixels breathe — the brand dither
// language at indicator scale. Pure CSS animation, 14px footprint.

const PERIMETER: ReadonlyArray<readonly [number, number]> = [
  [0, 0], [1, 0], [2, 0], [3, 0], [4, 0],
  [4, 1], [4, 2], [4, 3], [4, 4],
  [3, 4], [2, 4], [1, 4], [0, 4],
  [0, 3], [0, 2], [0, 1],
]

const CHASE_MS = 1600

export function LivePixels({ className }: { className?: string }) {
  return (
    <span className={`live-pixels ${className ?? ''}`} aria-hidden>
      {PERIMETER.map(([x, y], index) => (
        <span
          key={`${x}-${y}`}
          className="live-pixel"
          style={{
            left: x * 3,
            top: y * 3,
            animationDelay: `${(-index * CHASE_MS) / PERIMETER.length}ms`,
          }}
        />
      ))}
      <span className="live-pixel live-pixel-inner" style={{ left: 3, top: 6, animationDelay: '-900ms' }} />
      <span className="live-pixel live-pixel-inner" style={{ left: 9, top: 6, animationDelay: '-2100ms' }} />
      <span className="live-pixel live-pixel-core" style={{ left: 6, top: 6 }} />
    </span>
  )
}
