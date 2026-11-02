import { X } from 'lucide-react'
import { type CSSProperties, useCallback, useEffect, useState } from 'react'
import {
  alertLabel,
  alertSender,
  ISLAND_CAPSULE_WIDTH,
  ISLAND_EAR_WIDTH,
  type Alert,
  type AlertState,
  type IslandCommand,
  type IslandState,
} from '@shared/notifications'
import { BotIcon } from '@/components/bots/BotAvatar'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

// The island is black like the camera housing in every theme, so its marks
// keep the dark theme's colours, which hold their contrast on black.
const TONE: Record<AlertState, string> = {
  done: 'oklch(0.74 0.15 150)',
  reply: 'oklch(0.72 0.14 var(--hue))',
  needs_you: 'oklch(0.74 0.14 55)',
  stopped: 'oklch(0.7 0.18 25)',
}
const GLYPH: Record<AlertState, string> = {
  done: 'M5.2 8.3 7.1 10.2 10.9 6',
  reply: '',
  needs_you: 'M6.3 6.4a1.8 1.8 0 1 1 2.5 1.7c-.5.2-.8.6-.8 1.1v.3M8 11.4h0',
  stopped: 'M8 4.8v3.9M8 11.2h0',
}
// The concave curve where the island leaves the top edge, and the inset that
// keeps the capsule's shadow inside the window.
const FLARE = 6
const EASE = 'cubic-bezier(0.16, 1, 0.3, 1)'

// Keeps the last alerts while it retracts, so the shape closes over them.
export function NotchIsland() {
  const [state, setState] = useState<IslandState | null>(null)
  const [open, setOpen] = useState(false)
  useEffect(
    () =>
      window.jaz!.island.onState((next) => {
        setOpen(next.rows.length > 0)
        if (next.rows.length) setState(next)
      }),
    [],
  )
  return state && <Island state={state} open={open} />
}

// Who and what state sit in the ears beside the camera; what happened drops
// beneath once the ears have grown. Without a notch the same shape floats as
// a capsule.
function Island({ state: { rows, more, ears, notch }, open }: { state: IslandState; open: boolean }) {
  const { command } = window.jaz!.island
  const reduced = useReducedEffectsMotion()
  const [grown, setGrown] = useState(false)
  const [drop, setDrop] = useState(0)
  const measure = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => setDrop(node.offsetHeight))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  const top = rows[0]
  const sender = alertSender(top)
  const label = alertLabel(top)
  const band = notch?.height ?? 32
  const gap = notch?.width ?? 0
  const width = ears ? gap + 88 : (notch ? gap + 2 * ISLAND_EAR_WIDTH : ISLAND_CAPSULE_WIDTH) - 2 * FLARE
  const closed = notch ? gap - 2 * FLARE : band
  const dropped = !ears && (reduced || (open && grown))
  const radius = dropped ? 22 : notch ? 10 : band / 2
  // Reduced motion trades the morph for a crossfade, which `duration-150!`
  // keeps through the app's rule that zeroes every transition.
  const transition = reduced
    ? 'opacity 150ms'
    : open
      ? `width 240ms ${EASE}, height 280ms ${EASE}, border-radius 280ms ${EASE}, opacity 100ms`
      : 'all 220ms ease-in, opacity 80ms ease-in 140ms'
  return (
    <div
      onPointerEnter={() => command({ type: 'hover', inside: true })}
      onPointerLeave={() => command({ type: 'hover', inside: false })}
      onTransitionEnd={(event) => event.propertyName === 'width' && setGrown(open)}
      className={`group absolute top-0 left-1/2 w-(--width) -translate-x-1/2 text-white starting:w-(--closed) starting:opacity-0 ${reduced ? 'duration-150!' : ''} ${open ? '' : 'pointer-events-none opacity-0'}`}
      style={{ '--width': `${open || reduced ? width : closed}px`, '--closed': `${closed}px`, height: band + (dropped ? drop : 0), transition } as CSSProperties}
    >
      {notch && (
        <>
          <span className="absolute top-0 right-full size-1.5 bg-[radial-gradient(circle_at_0_100%,transparent_6px,#000_6.5px)]" />
          <span className="absolute top-0 left-full size-1.5 bg-[radial-gradient(circle_at_100%_100%,transparent_6px,#000_6.5px)]" />
        </>
      )}
      <div
        className="absolute inset-0 overflow-hidden bg-black shadow-[0_12px_24px_-10px_rgb(0_0_0/0.5)]"
        style={{ borderRadius: notch ? `0 0 ${radius}px ${radius}px` : radius, transition }}
      >
        <button
          type="button"
          aria-label={`${sender}, ${label}`}
          onClick={() => command({ type: 'open', sessionId: top.sessionId })}
          className="flex w-full justify-center text-[12px]"
          style={{ height: band }}
        >
          <span className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden pr-2 pl-3">
            <Identity row={top} size={20} live completion={grown && top.state === 'done' ? top.key : undefined} />
            {!ears && <span className="truncate font-medium">{sender}</span>}
          </span>
          <span className="shrink-0" style={{ width: gap }} />
          <span className="flex min-w-0 flex-1 items-center justify-end gap-1.5 overflow-hidden pr-3 pl-2 group-hover:invisible">
            <Mark state={top.state} />
            {!ears && (
              <span className="truncate font-semibold text-white/70" style={top.state === 'reply' ? undefined : { color: TONE[top.state] }}>
                {label}
              </span>
            )}
          </span>
        </button>
        <button
          type="button"
          aria-label="Dismiss"
          onClick={() => command({ type: 'dismiss', sessionId: top.sessionId })}
          className="invisible absolute top-0 right-1 grid w-8 place-items-center group-hover:visible"
          style={{ height: band }}
        >
          <span className="grid size-5 place-items-center rounded-full bg-white/15">
            <X size={10} strokeWidth={2.5} />
          </span>
        </button>
        {!ears && (
          <div ref={measure} className="absolute left-1/2 grid -translate-x-1/2 gap-2.5 px-[18px] pt-2.5 pb-4" style={{ top: band, width }}>
            {rows.map((row) => (
              <Row key={row.key} row={row} compact={rows.length > 1} command={command} />
            ))}
            {more > 0 && (
              <button type="button" onClick={() => command({ type: 'more' })} className="justify-self-start text-[12.5px] text-white/60 hover:text-white">
                +{more} more
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// In a burst each row carries its own face and mark, since the ears speak
// for the first alone.
function Row({ row, compact, command }: { row: Alert; compact: boolean; command: (command: IslandCommand) => void }) {
  const open = () => command({ type: 'open', sessionId: row.sessionId })
  return (
    <div className="flex flex-wrap items-start gap-x-2.5 gap-y-1.5">
      <button type="button" onClick={open} className="flex min-w-0 flex-1 items-start gap-2.5 text-left">
        {compact && <Identity row={row} size={18} />}
        <span className="grid min-w-0 flex-1 gap-0.5">
          {row.title && <span className="truncate text-[14px] font-semibold tracking-[-0.01em]">{row.title}</span>}
          <span
            className={`line-clamp-3 text-[13px] leading-[1.42] ${row.title ? 'text-white/70' : ''}`}
            style={{ WebkitLineClamp: (row.title ? 2 : 3) - (compact ? 1 : 0) }}
          >
            {row.body}
          </span>
        </span>
        {compact && row.state !== 'needs_you' && <Mark state={row.state} />}
      </button>
      {row.state === 'needs_you' && (
        <span className={`flex gap-2 ${compact ? '' : 'basis-full justify-end'}`}>
          <button
            type="button"
            onClick={() => command({ type: 'dismiss', sessionId: row.sessionId })}
            className="h-[26px] rounded-full bg-white/10 px-3 text-[12.5px] font-semibold hover:bg-white/15"
          >
            Later
          </button>
          <button type="button" onClick={open} className="h-[26px] rounded-full px-3 text-[12.5px] font-semibold text-black/80" style={{ background: TONE.needs_you }}>
            Answer
          </button>
        </span>
      )}
    </div>
  )
}

function Identity({ row, size, live, completion }: { row: Alert; size: number; live?: boolean; completion?: string }) {
  if (!row.avatars.length) {
    return <img src="./favicon.png" alt="" draggable={false} width={size} height={size} className="shrink-0" />
  }
  return <BotIcon avatars={row.avatars} size={size} mood={row.state === 'stopped' ? 'failed' : undefined} live={live} completion={completion} />
}

function Mark({ state }: { state: AlertState }) {
  return (
    <svg aria-hidden viewBox="0 0 16 16" className="size-4 shrink-0">
      <circle cx="8" cy="8" r={GLYPH[state] ? 8 : 4} style={{ fill: TONE[state] }} />
      <path d={GLYPH[state]} fill="none" stroke="#111" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
