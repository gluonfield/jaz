import { type ReactNode, type RefObject, createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useColorTheme } from '@/lib/appearanceScheme'
import { clientRuntime } from '@/lib/clientRuntime'
import { devPreview } from '@/lib/devPreview'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { OPEN_MS, RIM_RADIUS, between, bursting, clamp, createFrameLight, ease, lerp, mix, rim } from '@/lib/frameLight'
import { useTheme } from '@/lib/theme'

// The launch stage: night, with one line of light across the window and the
// wordmark risen behind it. The line is a frame with no height. For a card
// (first-run setup) it draws in and opens into the card's frame, lit as far
// round as setup has got; once nothing holds the stage, the frame bursts open
// to the window's rim onto the app.

// How far down the window the line lies.
const HORIZON = 0.46
const NIGHT = '#0b0a10'
const GROUND = [0.043, 0.039, 0.063]
const WHITE = [1, 0.96, 0.9]
const WARM = [1, 0.7, 0.48]
// The line is not left before it has been drawn and the name read.
const NAMED_MS = 1300
const DRAWN_MS = 400
const CARD_MS = 700
const REST_MS = 500

// `?entrance` plays the boot entrance in a browser dev build; it otherwise
// belongs to the desktop app's main window, where there is a boot to cover.
export const bootEntrance = clientRuntime.windowKind === 'main' && (clientRuntime.kind === 'electron' || devPreview('entrance') !== null)

type Card = { element: HTMLElement; progress: number; clip?: string }
type Hold = { card?: Card }

const Stage = createContext({ up: false, hold: (_: Hold) => () => {} })

export const useHorizonUp = () => useContext(Stage).up

// Raises the stage and keeps it up while mounted. Given a card, the frame
// holds it, lit `progress` (0..1) of the way round.
export function useHorizon(card?: RefObject<HTMLElement | null>, progress = 1) {
  const { hold } = useContext(Stage)
  const held = useRef<Hold>({})
  useLayoutEffect(() => {
    if (card?.current) held.current.card = { element: card.current, progress: 0 }
    return hold(held.current)
  }, [hold, card])
  useLayoutEffect(() => {
    if (held.current.card) held.current.card.progress = progress
  })
}

// What goes under the line, once it has been drawn: a status, or the choices a
// launch needs.
export function HorizonBelow({ children }: { children: ReactNode }) {
  useHorizon()
  return (
    <div
      className="night fixed inset-x-0 bottom-0 overflow-y-auto px-6 pb-8 text-ink transition-opacity delay-500 duration-500 starting:opacity-0"
      style={{ top: `calc(${HORIZON * 100}% + 24px)` }}
    >
      <div className="mx-auto flex w-full max-w-[340px] flex-col items-center">{children}</div>
    </div>
  )
}

// A wait: under the line where the stage is up, and otherwise late and quiet
// in the page, without raising the stage for it.
export function HorizonStatus({ children }: { children: ReactNode }) {
  return useHorizonUp() ? (
    <HorizonBelow>
      <p className="text-[13px] text-ink-3">{children}</p>
    </HorizonBelow>
  ) : (
    <p className="grid h-full place-items-center text-[13px] text-ink-3 transition-opacity delay-1000 duration-300 starting:opacity-0">{children}</p>
  )
}

const smooth = (k: number) => k * k * (3 - 2 * k)
const easeOut = (k: number) => 1 - (1 - k) ** 3

function rgb(color: string) {
  const pen = new OffscreenCanvas(1, 1).getContext('2d')!
  pen.fillStyle = color
  pen.fillRect(0, 0, 1, 1)
  return [...pen.getImageData(0, 0, 1, 1).data.slice(0, 3)].map((channel) => channel / 255)
}

// The theme as light: the nebula's cool tone, and the accent a frame is drawn
// in round a white card, where warm white would not show.
function readLook() {
  const style = getComputedStyle(document.documentElement)
  const hue = style.getPropertyValue('--hue')
  const tint = Number(style.getPropertyValue('--tint'))
  return {
    day: !document.documentElement.classList.contains('dark'),
    cool: rgb(`oklch(0.62 ${0.2 * tint} ${hue})`),
    accent: rgb(`oklch(0.76 ${0.17 * tint} ${hue})`),
  }
}

export function Horizon({ entrance = false, children }: { entrance?: boolean; children: ReactNode }) {
  const reduce = useReducedEffectsMotion()
  const { resolved } = useTheme()
  const color = useColorTheme()
  const holds = useRef(new Set<Hold>())
  const canvas = useRef<HTMLCanvasElement>(null)
  const word = useRef<HTMLSpanElement>(null)
  const look = useRef<ReturnType<typeof readLook>>(null)
  // up: the stage is on screen. held: something under it still needs it, so
  // it lies beneath that; let go, it covers the app until it has opened.
  const [up, setUp] = useState(entrance)
  const [held, setHeld] = useState(false)
  const [opening, setOpening] = useState(false)
  const booting = useRef(entrance)

  const hold = useCallback((kept: Hold) => {
    holds.current.add(kept)
    // Its card stays unseen until the frame opens onto it.
    if (kept.card) kept.card.element.style.opacity = '0'
    setUp(true)
    setHeld(true)
    return () => {
      holds.current.delete(kept)
      setHeld(holds.current.size > 0)
    }
  }, [])
  const stage = useMemo(() => ({ up, hold }), [up, hold])

  useEffect(() => {
    look.current = readLook()
  }, [resolved, color])

  useEffect(() => {
    if (!up) return
    const light = createFrameLight(canvas.current!)
    const span = (ms: number) => (reduce ? 1 : ms)
    let since = performance.now()
    let last = since
    let openAt: number | null = null
    // How far the line has become the card, and the card's unlit part dimmed.
    let carded = 0
    let rested = 0
    let day = 0
    let arc = 1
    let box = [0, 0, 0, 0]
    let corner = 0
    let spoken = ''
    let idle = false
    let moved = false
    let begun = false
    let frame = 0

    const tick = (now: number) => {
      frame = requestAnimationFrame(tick)
      const held = holds.current.size > 0
      // A settled card needs only its nebula drifting: half the frames do.
      idle = !idle && held && carded === 1 && rested === 1 && !moved
      if (idle) return
      const elapsed = now - last
      last = now
      const named = span(booting.current ? NAMED_MS : DRAWN_MS)
      if (!held) openAt ??= Math.max(now, since + (booting.current ? named : 0))
      else if (openAt !== null && now < openAt) openAt = null
      if (openAt !== null && now >= openAt + span(OPEN_MS)) {
        booting.current = false
        setOpening(false)
        if (!held) {
          cancelAnimationFrame(frame)
          setUp(false)
          return
        }
        since = now
        openAt = null
        carded = 0
        begun = false
      }
      const out = openAt === null ? 0 : between(now, openAt, span(OPEN_MS))
      if (out > 0 && !begun) setOpening(true)
      begun = out > 0
      const t = now - since

      const card = [...holds.current].find((hold) => hold.card)?.card
      if (openAt === null) carded = clamp(carded + (card && t >= named ? elapsed : -elapsed) / span(CARD_MS))
      rested = clamp(rested + (carded === 1 && openAt === null ? elapsed : -elapsed) / span(REST_MS))
      day = clamp(day + (look.current?.day ? elapsed : -elapsed) / span(240))
      if (card) {
        const bounds = card.element.getBoundingClientRect()
        const next = [bounds.left + bounds.width / 2 - window.innerWidth / 2, bounds.top + bounds.height / 2 - window.innerHeight / 2, bounds.width / 2, bounds.height / 2]
        moved = next.some((value, index) => value !== box[index])
        box = next
        corner = parseFloat(getComputedStyle(card.element).borderTopLeftRadius)
        arc = rested === 0 ? card.progress : lerp(arc, card.progress, 1 - Math.exp(-elapsed / 140))
      }

      const lineY = window.innerHeight * (HORIZON - 0.5)
      const narrowed = ease(between(carded * CARD_MS, 0, 320))
      const opened = easeOut(smooth(between(carded * CARD_MS, 140, 560)))
      const settled = ease(carded)
      // On a white card the frame sits just outside it, in the accent.
      const pad = 2.5 * day * opened
      const drawn = (window.innerWidth / 2 - 1.5) * easeOut(between(t, 60, span(800)))
      const shape = [box[0] * opened, lerp(lineY, box[1], opened), lerp(drawn, box[2] + pad, narrowed), (box[3] + pad) * opened]
      const opens = bursting(out)
      const rect = mix(rim(), shape, opens.closed)

      if (card) {
        const scale = (2 * box[3]) / card.element.offsetHeight || 1
        const top = (shape[1] - shape[3] - (box[1] - box[3])) / scale
        const bottom = (box[1] + box[3] - (shape[1] + shape[3])) / scale
        const clip = carded === 1 ? '' : `inset(${top}px 0 ${bottom}px round ${(corner * opened) / scale}px)`
        if (clip !== card.clip) {
          card.element.style.clipPath = clip
          card.element.style.opacity = clip && String(between(carded * CARD_MS, 200, 120))
        }
        card.clip = clip
      }
      const leaving = openAt === null ? 0 : between(now, openAt, span(100))
      const rise = reduce ? 0 : 114 * (1 - easeOut(between(t, 320, 900)))
      const say = `opacity:${(1 - between(carded * CARD_MS, 0, 100)) * (1 - leaving)};transform:translateY(${rise}px)`
      if (say !== spoken) word.current!.style.cssText = say
      spoken = say

      light?.draw(reduce ? 0 : t / 1000, {
        rect,
        radius: lerp(RIM_RADIUS, (corner + pad) * opened, opens.closed),
        veil: opens.glow,
        ground: GROUND,
        open: opens.open,
        glow: between(t, 0, span(140)) * opens.glow,
        bloom: opens.bloom,
        spread: opens.spread,
        dust: Math.max(opens.bloom, lerp(0.12, 0.22, settled) * opens.closed),
        field: lerp(easeOut(between(t, 150, span(1500))), 0.42, settled),
        horizon: [lerp(rect[1] - rect[3], lineY, opens.closed), 1 - settled],
        arc,
        rest: lerp(1, lerp(0.2, 0.09, day), ease(rested)),
        sweepRate: 2.2,
        cool: look.current?.cool,
        ink: mix(WHITE, look.current?.accent ?? WHITE, day * settled),
        tint: mix(WARM, look.current?.accent ?? WARM, day * settled),
        weight: lerp(1.1, 2.8, day * settled),
      })
    }
    frame = requestAnimationFrame(tick)
    const resize = () => light?.resize()
    window.addEventListener('resize', resize)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('resize', resize)
    }
  }, [up, reduce])

  return (
    <Stage.Provider value={stage}>
      {up ? (
        <div
          aria-hidden
          style={{
            backgroundColor: opening ? undefined : NIGHT,
            zIndex: held ? 0 : 120,
          }}
          className={`fixed inset-0 select-none ${opening ? 'pointer-events-none' : ''} ${booting.current ? '' : 'transition-opacity duration-200 starting:opacity-0'}`}
        >
          <canvas ref={canvas} className="size-full" />
          <div className="absolute inset-x-0 h-[112px] overflow-hidden" style={{ top: `calc(${HORIZON * 100}% - 116px)` }}>
            <span ref={word} className="block text-center text-[84px] font-semibold leading-[112px] tracking-[-0.04em] text-[#fff7ea] opacity-0">
              jaz
            </span>
          </div>
        </div>
      ) : null}
      {children}
    </Stage.Provider>
  )
}
