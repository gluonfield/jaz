import { motion, useReducedMotion } from 'motion/react'
import { SCANLINE_BACKGROUND, SCANLINE_MASK } from '@/components/ui/rainbow'
import type { Board } from '@/lib/api/types'

// Boards have no thumbnails, so each cover is a stock miniature dashboard
// seeded by the board id — stable across renders, distinct across boards.
type TileKind = 'stat' | 'bars' | 'spark' | 'rows'
type TileSpec = { span: 1 | 2; kind: TileKind }

// Each layout fills a 3×2 cover grid (column spans sum to 6).
const COVER_LAYOUTS: TileSpec[][] = [
  [{ span: 2, kind: 'bars' }, { span: 1, kind: 'stat' }, { span: 1, kind: 'rows' }, { span: 2, kind: 'spark' }],
  [{ span: 1, kind: 'stat' }, { span: 2, kind: 'spark' }, { span: 2, kind: 'bars' }, { span: 1, kind: 'rows' }],
  [{ span: 2, kind: 'stat' }, { span: 1, kind: 'bars' }, { span: 1, kind: 'spark' }, { span: 1, kind: 'rows' }, { span: 1, kind: 'bars' }],
  [{ span: 1, kind: 'rows' }, { span: 1, kind: 'bars' }, { span: 1, kind: 'stat' }, { span: 2, kind: 'spark' }, { span: 1, kind: 'stat' }],
]

const ACCENTS = ['var(--color-primary)', 'var(--color-accent)', 'var(--color-ok)', 'var(--color-running)']
const BAR_HEIGHTS = [35, 60, 45, 85, 55, 70]

function coverSeed(id: string): number {
  let hash = 7
  for (const char of id) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return Math.abs(hash)
}

export function BoardCover({ board }: { board: Board }) {
  const reduce = useReducedMotion()
  const seed = coverSeed(board.id)
  const layout = COVER_LAYOUTS[seed % COVER_LAYOUTS.length]
  return (
    <motion.div
      aria-hidden
      whileHover={reduce ? undefined : 'hover'}
      className="relative overflow-hidden rounded-card bg-bg p-2.5 ring-1 ring-border/60 transition-shadow duration-200 group-hover:shadow-[0_10px_28px_-14px_rgb(0_0_0/0.3)]"
      style={{
        backgroundImage: 'radial-gradient(var(--color-border) 1px, transparent 1px)',
        backgroundSize: '14px 14px',
      }}
    >
      <div className="grid aspect-[16/9.2] grid-cols-3 grid-rows-2 gap-1.5">
        {layout.map((tile, i) => (
          <CoverTile
            key={i}
            kind={tile.kind}
            span={tile.span}
            // The layout already consumed seed % 4; a same-modulus accent would
            // tie every layout to one leading color, so shift to fresh bits.
            accent={ACCENTS[(Math.floor(seed / COVER_LAYOUTS.length) + i) % ACCENTS.length]}
            seed={seed + i}
          />
        ))}
      </div>
      <motion.div
        className="pointer-events-none absolute inset-0"
        style={{ background: SCANLINE_BACKGROUND, maskImage: SCANLINE_MASK, WebkitMaskImage: SCANLINE_MASK, x: '-60%', opacity: 0 }}
        variants={{
          hover: { x: ['-60%', '60%'], opacity: [0, 0.85, 0.85, 0], transition: { duration: 0.6, ease: 'easeInOut' } },
        }}
      />
    </motion.div>
  )
}

function CoverTile({ kind, span, accent, seed }: { kind: TileKind; span: 1 | 2; accent: string; seed: number }) {
  const Body = { stat: StatTile, bars: BarsTile, spark: SparkTile, rows: RowsTile }[kind]
  return (
    <div className={`flex min-h-0 flex-col rounded-[5px] bg-surface p-2 ${span === 2 ? 'col-span-2' : ''}`}>
      <div className="flex shrink-0 items-center gap-1 pb-1.5">
        <span className="size-1 shrink-0 rounded-full" style={{ background: accent }} />
        <span className="h-1 w-8 rounded-full bg-ink/10" />
      </div>
      <div className="min-h-0 flex-1">
        <Body span={span} accent={accent} seed={seed} />
      </div>
    </div>
  )
}

type TileBodyProps = { span: 1 | 2; accent: string; seed: number }

function StatTile(_: TileBodyProps) {
  return (
    <div className="flex h-full flex-col justify-end gap-1">
      <div className="h-2.5 w-8 rounded-[3px] bg-ink/20" />
      <div className="h-1 w-12 max-w-full rounded-full bg-ink/10" />
    </div>
  )
}

function BarsTile({ span, accent, seed }: TileBodyProps) {
  const barCount = span === 2 ? 6 : 4
  return (
    <div className="flex h-full items-end gap-1">
      {Array.from({ length: barCount }, (_, i) => (
        <div
          key={i}
          className={`flex-1 rounded-t-[2px] ${i === seed % barCount ? '' : 'bg-ink/15'}`}
          style={{
            height: `${BAR_HEIGHTS[(seed + i) % BAR_HEIGHTS.length]}%`,
            ...(i === seed % barCount ? { background: accent } : {}),
          }}
        />
      ))}
    </div>
  )
}

function SparkTile({ accent, seed }: TileBodyProps) {
  return (
    <svg viewBox="0 0 100 28" className="h-full w-full" preserveAspectRatio="none">
      <path
        d={seed % 2 === 0
          ? 'M0 22 C 10 20, 16 12, 26 14 S 44 25, 54 18 S 70 4, 80 8 S 94 13, 100 9'
          : 'M0 16 C 12 20, 20 8, 32 12 S 50 22, 62 14 S 82 6, 100 12'}
        fill="none"
        stroke={accent}
        strokeWidth="2"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}

function RowsTile({ accent, seed }: TileBodyProps) {
  return (
    <div className="flex h-full flex-col justify-center gap-1.5">
      {[0, 1, 2].map((i) => (
        <div key={i} className="flex items-center gap-1">
          <span
            className={`size-1 shrink-0 rounded-full ${i === seed % 3 ? '' : 'bg-ink/25'}`}
            style={i === seed % 3 ? { background: accent } : undefined}
          />
          <span className={`h-1 rounded-full bg-ink/10 ${i === 1 ? 'w-3/5' : 'w-4/5'}`} />
        </div>
      ))}
    </div>
  )
}
