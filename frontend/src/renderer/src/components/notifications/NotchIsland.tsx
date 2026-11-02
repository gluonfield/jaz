import { X } from 'lucide-react'
import { type CSSProperties, useCallback, useEffect, useState } from 'react'
import { flushSync } from 'react-dom'
import {
  alertLabel,
  alertSender,
  ISLAND_CAPSULE_WIDTH,
  ISLAND_EAR_WIDTH,
  ISLAND_RETRACT_MS,
  type Alert,
  type AlertState,
  type IslandCommand,
  type IslandState,
} from '@shared/notifications'
import { BotIcon } from '@/components/bots/BotAvatar'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import './island.css'

// The island is black like the camera housing in every theme, so its states
// keep the dark theme's colours, which hold their contrast on black.
const TONE: Record<Exclude<AlertState, 'reply'>, string> = {
  done: 'oklch(0.74 0.15 150)',
  needs_you: 'oklch(0.74 0.14 55)',
  stopped: 'oklch(0.7 0.18 25)',
}
// The concave curve where the island leaves the top edge, and the inset that
// keeps the capsule's shadow inside the window.
const FLARE = 6
// A critically damped spring: the shape leaves the camera from rest and
// settles without overshoot.
const SPRING = 'cubic-bezier(0.2, 0.1, 0.25, 1)'

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

// Who sits in the left ear beside the camera and the state, if it needs a
// word, in the right; what happened drops beneath as the ears grow. A
// bot is named beside its face; a plain thread, which only Jaz runs, puts its
// title beside the Jaz icon instead. Both ears grow evenly to fit the title,
// so the island stays centred on the camera, and the card wraps to their
// width rather than widening them. Without a notch the same shape floats as a
// capsule.
function Island({ state: { rows, more, ears, notch }, open }: { state: IslandState; open: boolean }) {
  const { command } = window.jaz!.island
  const reduced = useReducedEffectsMotion()
  const [grown, setGrown] = useState(false)
  const [drop, setDrop] = useState(0)
  const [fit, setFit] = useState(0)
  const measure = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => setDrop(node.offsetHeight))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  // Applied before paint, so the shape never opens to a stale width.
  const measureBand = useCallback((node: HTMLButtonElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => flushSync(() => setFit(node.offsetWidth)))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  const top = rows[0]
  const sender = alertSender(top)
  const label = alertLabel(top)
  const band = notch?.height ?? 32
  const gap = notch?.width ?? 0
  const width = (notch ? gap + 2 * ISLAND_EAR_WIDTH : ISLAND_CAPSULE_WIDTH) - 2 * FLARE
  const closed = notch ? gap - 2 * FLARE : band
  const shown = open || reduced
  // The ears match beside a camera, or when they hold all there is; a
  // capsule's title takes what its state leaves.
  const even = notch !== null || ears
  // How far the card hangs below the band; its rounder corners come only with
  // it, so the island leaves the camera in the notch's own shape.
  const card = !ears && shown ? drop : 0
  const radius = card ? 22 : notch ? 10 : band / 2
  // One spring carries the shape, the card trailing the ears out and leading
  // them back in; content and shadow fade in once it has left the camera.
  // Beside a notch the shape stays opaque, as it is closed only under the
  // camera housing; the floating capsule fades instead. Reduced motion trades
  // the morph for a crossfade, which `duration-150!` keeps through the app's
  // rule that zeroes every transition.
  const fade = notch ? 0 : 150
  const close = ISLAND_RETRACT_MS
  const transition = reduced
    ? 'opacity 150ms'
    : open
      ? `width 560ms ${SPRING}, height 560ms ${SPRING} 80ms, border-radius 560ms ${SPRING} 80ms, --island-open 360ms ${SPRING} 120ms, opacity ${fade}ms`
      : `width ${close}ms ${SPRING}, height ${close - 80}ms ${SPRING}, border-radius ${close - 80}ms ${SPRING}, --island-open 120ms, opacity ${fade}ms ${close - fade}ms`
  return (
    <div
      onPointerEnter={() => command({ type: 'hover', inside: true })}
      onPointerLeave={() => command({ type: 'hover', inside: false })}
      onTransitionEnd={(event) => event.propertyName === 'width' && setGrown(open)}
      className={`group absolute top-0 left-1/2 w-(--width) -translate-x-1/2 text-white starting:w-(--closed) starting:opacity-0 starting:[--island-open:0] ${reduced ? 'duration-150!' : ''} ${open ? '' : 'pointer-events-none opacity-0'} ${shown ? '[--island-open:1]' : ''}`}
      style={
        {
          '--width': `${shown ? fit : closed}px`,
          '--closed': `${closed}px`,
          height: band + card,
          borderRadius: notch ? `0 0 ${radius}px ${radius}px` : radius,
          transition,
        } as CSSProperties
      }
    >
      {notch && (
        <>
          <span className="absolute top-0 right-full size-1.5 bg-[radial-gradient(circle_at_0_100%,transparent_6px,#000_6.5px)]" />
          <span className="absolute top-0 left-full size-1.5 bg-[radial-gradient(circle_at_100%_100%,transparent_6px,#000_6.5px)]" />
        </>
      )}
      <div className="absolute inset-0 overflow-hidden rounded-[inherit] bg-black shadow-[0_12px_24px_-10px_rgb(0_0_0/calc(var(--island-open)*0.5))] *:opacity-(--island-open) *:blur-[calc((1-var(--island-open))*4px)]">
        <div className="absolute top-0 left-1/2 grid -translate-x-1/2" style={{ width: even ? 'max-content' : width, minWidth: ears ? 0 : width }}>
          <button
            ref={measureBand}
            type="button"
            aria-label={label ? `${sender}, ${label}` : sender}
            onClick={() => command({ type: 'open', sessionId: top.sessionId })}
            className="grid text-[12px]"
            style={{ height: band, gridTemplateColumns: `1fr ${gap}px ${even ? '1fr' : 'max-content'}` }}
          >
            <span className="flex min-w-0 items-center gap-1.5 overflow-hidden pr-2 pl-3">
              <Identity row={top} size={20} live completion={grown && top.state === 'done' ? top.key : undefined} />
              {!ears && <span className={`truncate font-medium ${notch ? 'max-w-48' : ''}`}>{top.name || top.title}</span>}
            </span>
            <span className="col-start-3 flex min-w-9 items-center justify-end pr-3 pl-2 group-hover:invisible">
              <State row={top} />
            </span>
          </button>
          {!ears && (
            <div ref={measure} className="grid gap-2.5 px-[18px] pt-2.5 pb-4 contain-inline-size">
              {rows.map((row, index) => (
                <Row key={row.key} row={index || row.name ? row : { ...row, title: '' }} compact={rows.length > 1} command={command} />
              ))}
              {more > 0 && (
                <button type="button" onClick={() => command({ type: 'more' })} className="justify-self-start text-[12.5px] text-white/60 hover:text-white">
                  +{more} more
                </button>
              )}
            </div>
          )}
        </div>
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
        {compact && row.state !== 'needs_you' && <State row={row} />}
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

function State({ row }: { row: Alert }) {
  if (row.state === 'reply') return null
  return (
    <span className="text-[12px] leading-5 font-semibold whitespace-nowrap" style={{ color: TONE[row.state] }}>
      {alertLabel(row)}
    </span>
  )
}
