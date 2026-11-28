import { getAppearance } from '@/lib/appearance'
import { GROUND, HORIZON, OPEN_MS, RIM_RADIUS, between, bursting, createFrameLight, ease, easeOut, lerp, mix, rgb, rim } from '@/lib/frameLight'

// The window has closed onto the line by now.
const SHUT_MS = 480
// The new workspace is not opened before its name has been readable.
const NAMED_MS = 1380
// The new workspace gets this long to draw before the line opens onto it.
const SETTLE_MS = 120
const CANCEL_FADE_MS = 240
// How far apart neighbouring workspaces' lamps lie, in window widths.
const GAP = 0.38
const TURN_AT_MS = 260
const TURN_MS = 580
const NAME_AT_MS = 680
// The shader draws this many lamps, the nearest ones.
const LAMPS = 4
const RIPPLE_MS = 1200

export interface Passage {
  // Settles once the window has closed over the workspace being left; only then may the app change.
  shut: Promise<void>
  // Call when the app shows the destination. Settles when the line has opened onto it.
  arrive: () => Promise<void>
  // Ends the passage at once, back on the app as it stands.
  cancel: () => void
}

const STILL: Passage = { shut: Promise.resolve(), arrive: () => Promise.resolve(), cancel: () => {} }
const wait = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms))

// A workspace's own colour of light, the same every time for its id.
const hueOf = (id: string) => [...id].reduce((sum, letter) => (sum * 31 + letter.charCodeAt(0)) >>> 0, 7) % 360

// The passage between workspaces, on the launch stage: the window closes onto
// its line of light, where each of `places` (in the menu's order) burns as a
// lamp in its own colour. The horizon turns from the lamp at `from` to the one
// at `to`, whose name rises from behind the line as the wordmark does at
// launch, and the line bursts open onto the new workspace. Reduced motion, or
// a browser that cannot draw the light, switches without it.
export function turnToWorkspace(places: { id: string; name: string }[], from: number, to: number): Passage {
  if (!getAppearance().effects || matchMedia('(prefers-reduced-motion: reduce)').matches) return STILL
  const stage = document.createElement('div')
  stage.role = 'status'
  stage.ariaLabel = `Switching to ${places[to].name}`
  stage.className = 'fixed inset-0 [-webkit-app-region:no-drag]'
  stage.style.zIndex = 'calc(var(--z-modal) + 2)'
  const canvas = stage.appendChild(document.createElement('canvas'))
  canvas.className = 'size-full'
  const behind = stage.appendChild(document.createElement('div'))
  behind.className = 'absolute inset-x-0 flex h-[112px] items-end justify-center overflow-hidden'
  behind.style.top = `calc(${HORIZON * 100}% - 116px)`
  const word = behind.appendChild(document.createElement('span'))
  word.className = 'origin-bottom whitespace-nowrap text-[84px] font-semibold leading-[112px] tracking-[-0.04em] text-[#fff7ea] opacity-0'
  word.textContent = places[to].name
  document.body.append(stage)
  const light = createFrameLight(canvas)
  if (!light) {
    stage.remove()
    return STILL
  }
  // A long name is drawn smaller to fit the window.
  const fit = Math.min(1, (0.84 * innerWidth) / word.offsetWidth)
  const tint = Number(getComputedStyle(document.documentElement).getPropertyValue('--tint'))
  const tones = places.map((place) => rgb(`oklch(0.7 ${0.19 * tint} ${hueOf(place.id)})`))

  const since = performance.now()
  let opens = Infinity
  let frame = 0
  const tick = () => {
    frame = requestAnimationFrame(tick)
    const now = performance.now()
    const t = now - since
    const burst = bursting(between(now, opens, OPEN_MS))
    const lineY = innerHeight * (HORIZON - 0.5)
    const shut = ease(between(t, 40, SHUT_MS - 40)) * burst.closed
    const rect = mix(rim(), [0, lineY, innerWidth / 2 - 1.5, 0], shut)
    const glow = between(t, 0, 140) * burst.glow
    const gap = GAP * innerWidth
    const turn = between(t, TURN_AT_MS, TURN_MS)
    const turned = ease(turn)
    const at = lerp(from, to, turned)
    // The lamps streak while the horizon turns fastest.
    const stretch = Math.min(1 + (42 * Math.abs(to - from) * gap * Math.min(turn, 1 - turn) ** 2) / TURN_MS, 9)
    const lit = between(t, 340, 160) * burst.closed
    const flare = 0.5 * Math.exp(-(((t - 860) / 150) ** 2))
    const bright = (place: number) => (place === to ? lerp(0.45, 1, turned) + flare : place === from ? lerp(1, 0.45, turned) : 0.45)
    const nearest = places
      .map((_, place) => place)
      .sort((a, b) => Math.abs(a - at) - Math.abs(b - at))
      .slice(0, LAMPS)
    // One ripple as the name rises, then one after another while the workspace is awaited.
    const beat = t < NAMED_MS ? between(t, 840, 520) : ((t - NAMED_MS) % RIPPLE_MS) / RIPPLE_MS
    light.draw(t / 1000, {
      rect,
      radius: lerp(RIM_RADIUS, 0, shut),
      veil: glow,
      ground: GROUND,
      open: 1,
      glow,
      bloom: burst.bloom,
      spread: burst.spread,
      dust: Math.max(burst.bloom, 0.16 * shut),
      field: easeOut(between(t, 260, 1100)),
      horizon: [lerp(rect[1] - rect[3], lineY, burst.closed), 1],
      sweepRate: 2.2,
      cool: tones[to],
      pan: at * gap,
      lamps: nearest.flatMap((place) => [(place - at) * gap, lit * bright(place), stretch]),
      tones: nearest.flatMap((place) => tones[place]),
      pulse: [easeOut(beat) * (innerWidth / 2), Math.sin(Math.PI * beat) * burst.closed],
    })
    word.style.opacity = String(Number(t >= NAME_AT_MS) * (1 - between(now, opens, 100)))
    word.style.transform = `translateY(${114 * (1 - easeOut(between(t, NAME_AT_MS, 460)))}px) scale(${fit})`
  }
  tick()
  const end = () => {
    cancelAnimationFrame(frame)
    stage.remove()
  }

  return {
    shut: wait(SHUT_MS),
    // Timers pace the passage, so it still ends in a window too hidden to draw frames.
    arrive: async () => {
      opens = Math.max(since + NAMED_MS, performance.now() + SETTLE_MS)
      await wait(opens + OPEN_MS - performance.now())
      end()
    },
    cancel: () => void stage.animate({ opacity: 0 }, CANCEL_FADE_MS).finished.then(end),
  }
}
