import { Plus } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import type { ReactNode } from 'react'
import { SCANLINE_BACKGROUND, SCANLINE_MASK } from '@/components/ui/rainbow'

// A miniature mission control for the new-board modal: live-looking tiles on
// the dotted canvas plus a free cell, with the rainbow scanline sweeping
// fresh data into the first tile.
export function BoardIllustration() {
  const reduce = useReducedMotion()
  const tileIn = (delay: number) =>
    reduce
      ? {}
      : {
          initial: { opacity: 0, y: 10, scale: 0.97 },
          animate: { opacity: 1, y: 0, scale: 1 },
          transition: { delay, duration: 0.35, ease: [0.22, 1, 0.36, 1] as const },
        }
  return (
    <div
      aria-hidden
      className="rounded-card bg-bg p-3 ring-1 ring-border"
      style={{
        backgroundImage: 'radial-gradient(var(--color-border) 1px, transparent 1px)',
        backgroundSize: '16px 16px',
      }}
    >
      <div className="grid grid-cols-3 gap-2.5">
        <motion.div {...tileIn(0.05)} className="col-span-2 h-[108px]">
          <PullRequestsTile />
        </motion.div>
        <motion.div {...tileIn(0.14)} className="h-[108px]">
          <DeploysTile />
        </motion.div>
        <motion.div {...tileIn(0.23)} className="h-[96px]">
          <TokensTile />
        </motion.div>
        <motion.div {...tileIn(0.3)} className="h-[96px]">
          <InboxTile />
        </motion.div>
        <motion.div {...tileIn(0.4)} className="h-[96px]">
          <div className="flex h-full items-center justify-center gap-1.5 rounded-card border border-dashed border-border text-[11px] text-ink-3">
            <Plus size={12} />
            New widget
          </div>
        </motion.div>
      </div>
    </div>
  )
}

function TileFrame({
  title,
  time,
  dot,
  children,
}: {
  title: string
  time: string
  dot?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-card bg-surface ring-1 ring-border">
      <div className="flex h-7 shrink-0 items-center gap-1.5 px-2.5">
        {dot ?? <span className="size-1.5 shrink-0 rounded-full bg-primary" />}
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-ink">{title}</span>
        <span className="shrink-0 text-[10px] tabular-nums text-ink-3">{time}</span>
      </div>
      <div className="min-h-0 flex-1 px-2.5 pb-2.5">{children}</div>
    </div>
  )
}

// The fresh-data moment from the real board, looping every few seconds.
function Scanline() {
  return (
    <motion.div
      className="pointer-events-none absolute inset-0 z-10"
      style={{
        background: SCANLINE_BACKGROUND,
        maskImage: SCANLINE_MASK,
        WebkitMaskImage: SCANLINE_MASK,
      }}
      initial={{ x: '-60%', opacity: 0 }}
      animate={{ x: ['-60%', '-24%', '24%', '60%'], opacity: [0, 0.85, 0.85, 0] }}
      transition={{
        delay: 1.2,
        duration: 0.7,
        ease: 'easeInOut',
        times: [0, 0.12, 0.82, 1],
        repeat: Infinity,
        repeatDelay: 4.5,
      }}
    />
  )
}

function PullRequestsTile() {
  const reduce = useReducedMotion()
  const bars = [35, 60, 45, 85, 65, 52, 74]
  return (
    <div className="relative h-full overflow-hidden rounded-card">
      <TileFrame title="Open PRs" time="2m">
        <div className="flex h-full items-end gap-5 pb-0.5">
          <div className="shrink-0">
            <div className="text-[20px] font-semibold leading-tight tabular-nums text-ink">7</div>
            <div className="text-[10px] text-ink-2">open</div>
            <div className="text-[10px] font-medium text-ok">+2 today</div>
          </div>
          <div className="flex h-12 flex-1 items-end gap-1">
            {bars.map((height, index) => (
              <motion.div
                key={index}
                initial={reduce ? false : { height: 0 }}
                animate={{ height: `${height}%` }}
                transition={{ delay: 0.4 + index * 0.06, duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
                className={`flex-1 rounded-t-[3px] ${index === 4 ? 'bg-accent' : 'bg-primary/80'}`}
              />
            ))}
          </div>
        </div>
      </TileFrame>
      {reduce ? null : <Scanline />}
    </div>
  )
}

function DeploysTile() {
  const services = [
    { name: 'api', detail: 'v214', dot: 'bg-ok' },
    { name: 'web', detail: 'v213', dot: 'bg-ok' },
    { name: 'worker', detail: 'deploying', dot: 'bg-running animate-pulse' },
  ]
  return (
    <TileFrame
      title="Deploys"
      time="now"
      dot={<span className="size-1.5 shrink-0 animate-pulse rounded-full bg-running" />}
    >
      <div className="flex h-full flex-col justify-center gap-1.5">
        {services.map((service) => (
          <div key={service.name} className="flex items-center gap-1.5">
            <span className={`size-1.5 shrink-0 rounded-full ${service.dot}`} />
            <span className="min-w-0 flex-1 truncate text-[11px] text-ink-2">{service.name}</span>
            <span className="shrink-0 text-[10px] tabular-nums text-ink-3">{service.detail}</span>
          </div>
        ))}
      </div>
    </TileFrame>
  )
}

function TokensTile() {
  const reduce = useReducedMotion()
  return (
    <TileFrame title="Tokens" time="5m">
      <div className="flex h-full flex-col justify-end gap-1 pb-0.5">
        <div className="flex items-baseline gap-1.5">
          <span className="text-[15px] font-semibold tabular-nums text-ink">312k</span>
          <span className="text-[10px] font-medium text-ok">−18%</span>
        </div>
        <svg viewBox="0 0 100 28" className="h-7 w-full" preserveAspectRatio="none">
          <motion.path
            d="M0 22 C 10 20, 16 12, 26 14 S 44 25, 54 18 S 70 4, 80 8 S 94 13, 100 9"
            fill="none"
            stroke="var(--color-primary)"
            strokeWidth="2"
            strokeLinecap="round"
            vectorEffect="non-scaling-stroke"
            initial={reduce ? false : { pathLength: 0 }}
            animate={{ pathLength: 1 }}
            transition={{ delay: 0.5, duration: 0.9, ease: 'easeOut' }}
          />
        </svg>
      </div>
    </TileFrame>
  )
}

function InboxTile() {
  const rows = [
    { text: '2 PRs need review', dot: 'bg-accent' },
    { text: 'CI green on main', dot: 'bg-ok' },
    { text: '1 flaky test', dot: 'bg-danger' },
  ]
  return (
    <TileFrame title="Inbox triage" time="12m">
      <div className="flex h-full flex-col justify-center gap-1.5">
        {rows.map((row) => (
          <div key={row.text} className="flex items-center gap-1.5">
            <span className={`size-1.5 shrink-0 rounded-full ${row.dot}`} />
            <span className="min-w-0 flex-1 truncate text-[11px] text-ink-2">{row.text}</span>
          </div>
        ))}
      </div>
    </TileFrame>
  )
}
