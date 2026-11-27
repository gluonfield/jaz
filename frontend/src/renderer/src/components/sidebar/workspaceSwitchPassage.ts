import { OPEN_MS, RIM_RADIUS, between, bursting, clamp, createFrameLight, ease, lerp, mix, rim } from '@/lib/frameLight'

const LIGHT_MS = 140
const GATHER_AT_MS = 100
const GATHER_MS = 600
// How far behind the frame each of its three echoes runs, and how bright.
const ECHO_MS = 55
const ECHO_LUM = [0.42, 0.31, 0.2]
const TILE_RADIUS = 16

// Draws a frame of light round the window on `canvas`, which lies over it, and
// gathers it onto a tile of `tile` CSS pixels at the centre, trailing echoes
// while a scrim dims the page beneath. `arrive` bursts the frame back out over
// OPEN_MS; `since` is when it began. Returns null where WebGL cannot do it.
export function runPassage(canvas: HTMLCanvasElement, tile: [number, number]) {
  const light = createFrameLight(canvas)
  if (!light) return null
  const from = rim()
  const onto = [0, 0, tile[0] / 2, tile[1] / 2]

  const since = performance.now()
  let arrived = 0
  let frame = 0
  const gathered = (now: number) => ease(between(now, since + GATHER_AT_MS, GATHER_MS)) * bursting(arrived ? between(now, arrived, OPEN_MS) : 0).closed
  const draw = (now: number) => {
    const burst = bursting(arrived ? between(now, arrived, OPEN_MS) : 0)
    const gather = gathered(now)
    const echoes = [1, 2, 3].map((behind) => gathered(now - behind * ECHO_MS))
    const glow = between(now, since, LIGHT_MS) * burst.glow
    light.draw((now - since) / 1000, {
      rect: mix(from, onto, gather),
      radius: lerp(RIM_RADIUS, TILE_RADIUS, gather),
      echoA: mix(from, onto, echoes[0]),
      echoB: mix(from, onto, echoes[1]),
      echoC: mix(from, onto, echoes[2]),
      echoRadius: echoes.map((behind) => lerp(RIM_RADIUS, TILE_RADIUS, behind)),
      echoLum: echoes.map((behind, index) => ECHO_LUM[index] * clamp(Math.abs(behind - gather) * 7)),
      veil: 0.84 * glow,
      open: burst.open,
      glow,
      bloom: burst.bloom,
      spread: burst.spread,
      dust: Math.max(burst.bloom, 0.5 * glow * gather),
    })
    frame = requestAnimationFrame(draw)
  }
  draw(since)

  return {
    since,
    arrive: () => {
      arrived = performance.now()
    },
    stop: () => cancelAnimationFrame(frame),
  }
}
