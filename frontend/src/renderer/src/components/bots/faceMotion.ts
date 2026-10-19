import type { BotMood } from '@/lib/bots'

// One animation clock for every live bot face on screen. Each frame works
// out a pose per face and writes it straight into its SVG, without rendering React.
// Faces off screen, in a hidden window or under reduced motion hold still.

export type EyeKind = 'open' | 'happy' | 'worried' | 'sleepy'

// Where a face's parts sit on its 100×100 grid.
export type FaceGeometry = { eyeLine: number; bottom: number }

// The eyes' centres; drawn right of centre, so the face glances aside.
const EYE_X = [47, 67] as const

// Faces this large follow the cursor; smaller ones glance around instead of
// turning their heads.
const FOLLOW_SIZE = 40

export function eyePath(kind: EyeKind, cx: number, cy: number): string {
  if (kind === 'happy') {
    return `M${cx - 6} ${cy + 3}q6 -10 12 0`
  }
  if (kind === 'worried') {
    return cx < 57 ? `M${cx + 4} ${cy - 3}L${cx - 4} ${cy + 3}` : `M${cx - 4} ${cy - 3}L${cx + 4} ${cy + 3}`
  }
  if (kind === 'sleepy') {
    return `M${cx - 5} ${cy + 3}h10`
  }
  return `M${cx - 2} ${cy - 7}l4 14`
}

export function restingEyes(mood: BotMood): EyeKind {
  if (mood === 'failed') {
    return 'worried'
  }
  return mood === 'away' ? 'sleepy' : 'open'
}

// A pose in face units. `yaw` turns the head (-1 to 1), sliding and narrowing
// the eyes across a sphere; the light moves against it.
type Mark = [x: number, y: number, opacity: number]

type Pose = {
  x: number
  y: number
  tilt: number
  sx: number
  sy: number
  lookX: number
  lookY: number
  open: number
  yaw: number
  light: [number, number]
  eyes: EyeKind
  ring: { r: number; opacity: number }
  dots: Mark[]
  zs: Mark[]
}

function still(eyes: EyeKind): Pose {
  return { x: 0, y: 0, tilt: 0, sx: 1, sy: 1, lookX: 0, lookY: 0, open: 1, yaw: 0, light: [0, 0], eyes, ring: { r: 50, opacity: 0 }, dots: [], zs: [] }
}

type Clip = 'nod' | 'cheer'

const CLIP_LENGTH: Record<Clip, number> = { nod: 1.3, cheer: 2.4 }

// The move a change of mood plays once before the new mood's loop: a nod as
// work starts, a happy hop as it finishes.
function clipFor(from: BotMood, to: BotMood): Clip | null {
  const busy = (mood: BotMood) => mood === 'working' || mood === 'thinking'
  if (busy(to) && !busy(from)) {
    return 'nod'
  }
  return null
}

type LiveFace = {
  svg: SVGSVGElement
  body: SVGGElement
  eyes: SVGPathElement[]
  highlight: SVGElement | null
  shade: SVGElement | null
  ring: SVGCircleElement
  dots: SVGCircleElement[]
  zs: SVGTextElement[]
  geometry: FaceGeometry
  follow: boolean
  seed: number
  mood: BotMood
  since: number
  clip: { name: Clip; start: number } | null
  visible: boolean
  // Where the face is on screen, measured at most every BOX_AGE ms while the
  // cursor moves, and the eased direction it looks toward the cursor.
  box: { rect: DOMRect; at: number } | null
  look: [number, number]
  drawn: EyeKind
}

function smooth(edge0: number, edge1: number, value: number): number {
  const k = Math.min(1, Math.max(0, (value - edge0) / (edge1 - edge0)))
  return k * k * (3 - 2 * k)
}

// 0 outside [from, to], rising to 1 in its middle.
function bump(value: number, from: number, to: number): number {
  if (value < from || value > to) {
    return 0
  }
  return Math.sin((Math.PI * (value - from)) / (to - from))
}

// A looping keyframe track of [time, value] pairs, eased between keys.
function track(keys: [number, number][], time: number): number {
  const period = keys[keys.length - 1][0]
  const at = time % period
  for (let i = 1; i < keys.length; i++) {
    if (at <= keys[i][0]) {
      const [t0, v0] = keys[i - 1]
      const [t1, v1] = keys[i]
      return v0 + (v1 - v0) * smooth(t0, t1, at)
    }
  }
  return keys[0][1]
}

// How closed the eyes are: a blink every few seconds, every third a double.
function blink(time: number, seed: number): number {
  const period = 3.4 + seed * 0.5
  const local = (time + seed) % period
  const first = bump(local, 0, 0.18)
  return Math.floor((time + seed) / period) % 3 === 2 ? Math.max(first, bump(local, 0.26, 0.44)) : first
}

const TURN: [number, number][] = [[0, 0], [1.6, 0], [2.5, 0.85], [4.1, 0.85], [5.3, -0.8], [6.9, -0.8], [7.9, 0], [9.2, 0]]
const GLANCE: [number, number][] = [[0, 0], [1.2, 0], [1.5, 5], [2.6, 5], [2.9, -5], [4.0, -5], [4.3, 0], [5.6, 0]]
const WORK_GLANCE: [number, number][] = [[0, 0], [1.4, 0], [1.6, 3], [2.6, 3], [2.8, -2], [3.6, -2], [3.8, 0], [4.4, 0]]

function breathe(pose: Pose, time: number) {
  const breath = Math.sin((time * Math.PI) / 2)
  pose.sy = 1 + 0.035 * breath
  pose.sx = 1 - 0.02 * breath
}

// Idle: a large face follows the cursor while it moves, with the light
// following too, and otherwise turns its head now and then; a small one
// glances around.
function idlePose(face: LiveFace, pose: Pose, time: number, following: boolean) {
  breathe(pose, time)
  if (following) {
    const [px, py] = face.look
    pose.lookX = px * 6
    pose.lookY = py * 4
    pose.tilt = px * 5
    pose.light = [px * 12, py * 9]
  } else if (face.follow) {
    const pitch = Math.sin(time * 0.6 + face.seed * 5) * 0.3
    pose.yaw = track(TURN, time * 1.3 + face.seed * 11) * 0.7
    pose.lookY = pitch * 6
    pose.light = [-pose.yaw * 11, -pitch * 7]
    pose.sx *= 1 - 0.03 * Math.abs(pose.yaw)
  } else {
    pose.lookX = track(GLANCE, time + face.seed * 7)
  }
}

function moodPose(face: LiveFace, time: number, following: boolean): Pose {
  const local = time - face.since
  const pose = still(restingEyes(face.mood))
  pose.open = 1 - 0.9 * blink(time, face.seed)
  if (face.mood === 'working') {
    const at = local % 0.8
    if (at < 0.16) {
      const k = Math.sin((Math.PI * at) / 0.16)
      pose.sy = 1 - 0.1 * k
      pose.sx = 1 + 0.07 * k
    } else {
      const k = Math.sin((Math.PI * (at - 0.16)) / 0.64)
      pose.y = -10 * k
      pose.sy = 1 + 0.04 * k
      pose.sx = 1 - 0.03 * k
    }
    pose.open = 0.8
    pose.lookX = track(WORK_GLANCE, local)
  } else if (face.mood === 'thinking') {
    pose.lookX = 4
    pose.lookY = -5
    pose.tilt = Math.sin(local * 1.3) * 5
    const angle = local * 2.4
    pose.dots = [0, 2.6].map((phase) => [50 + 62 * Math.cos(angle + phase), 52 + 18 * Math.sin(angle + phase), 1])
  } else if (face.mood === 'waiting') {
    const at = local % 1.9
    const lean = 0.05 * bump(at, 0, 0.5)
    pose.sx = 1 + lean
    pose.sy = 1 + lean
    pose.y = -3 * bump(at, 0, 0.22) - 3 * bump(at, 0.26, 0.5)
    pose.lookX = -7
    pose.open = 1.15
    pose.ring = { r: 48 + 22 * (at / 1.9), opacity: 0.55 * (1 - at / 1.9) }
  } else if (face.mood === 'failed') {
    const at = local % 2.6
    pose.y = 3
    pose.sy = 0.97
    pose.lookY = 2
    pose.open = 1
    if (at < 0.6) {
      pose.x = Math.sin(at * 42) * 3.5 * (1 - at / 0.6)
    }
  } else if (face.mood === 'away') {
    pose.y = Math.sin(time * 0.9) * 2.5
    pose.sy = 1 + 0.03 * Math.sin(time * 0.9)
    pose.open = 1
    pose.zs = [0, 1].map((i) => {
      const at = (time * 0.45 + i * 0.5) % 1
      return [82 + at * 14 + i * 4, 22 - at * 34, Math.sin(Math.PI * at)]
    })
  } else {
    idlePose(face, pose, time, following)
  }
  return pose
}

function clipPose(face: LiveFace, clip: Clip, at: number, time: number): Pose {
  if (clip === 'nod') {
    const nod = still('open')
    breathe(nod, time)
    nod.sy *= 1 - 0.07 * bump(at, 0, 0.16)
    nod.lookY = 4 * bump(at, 0.14, 0.5)
    nod.y = 2 * bump(at, 0.14, 0.5)
    nod.open = at > 1.2 ? 1 - 0.9 * blink(time, face.seed) : 1 + 0.3 * bump(at, 0.45, 1)
    return nod
  }
  const cheer = still('happy')
  cheer.sy = 1 - 0.09 * bump(at, 0, 0.14) - 0.09 * bump(at, 0.56, 0.7)
  cheer.sx = 1 + 0.06 * bump(at, 0, 0.14) + 0.06 * bump(at, 0.56, 0.7)
  cheer.y = -13 * bump(at, 0.14, 0.56)
  return cheer
}

function place(marks: SVGElement[], positions: Mark[]) {
  marks.forEach((mark, index) => {
    const [x, y, opacity] = positions[index] ?? [0, 0, 0]
    mark.setAttribute('transform', `translate(${x} ${y})`)
    mark.setAttribute('opacity', String(opacity))
  })
}

function apply(face: LiveFace, pose: Pose) {
  const { eyeLine, bottom } = face.geometry
  face.body.setAttribute(
    'transform',
    `translate(${pose.x} ${pose.y}) rotate(${pose.tilt} 50 ${bottom}) translate(50 ${bottom}) scale(${pose.sx} ${pose.sy}) translate(-50 ${-bottom})`,
  )
  if (pose.eyes !== face.drawn) {
    face.eyes.forEach((eye, index) => eye.setAttribute('d', eyePath(pose.eyes, EYE_X[index], eyeLine)))
    face.drawn = pose.eyes
  }
  face.eyes.forEach((eye, index) => {
    const cx = EYE_X[index]
    const angle = Math.asin((cx - 50) / 40)
    const turned = angle + pose.yaw * 0.6
    const slide = 40 * (Math.sin(turned) - Math.sin(angle))
    const narrow = Math.max(0.25, Math.cos(turned) / Math.cos(angle))
    eye.setAttribute(
      'transform',
      `translate(${cx + slide + pose.lookX} ${eyeLine + pose.lookY}) scale(${narrow} ${pose.open}) translate(${-cx} ${-eyeLine})`,
    )
  })
  face.highlight?.setAttribute('transform', `translate(${pose.light[0]} ${pose.light[1]})`)
  face.shade?.setAttribute('transform', `translate(${-pose.light[0] * 0.7} ${-pose.light[1] * 0.7})`)
  face.ring.setAttribute('r', String(pose.ring.r))
  face.ring.setAttribute('opacity', String(pose.ring.opacity))
  place(face.dots, pose.dots)
  place(face.zs, pose.zs)
}

// Hands the face back as React last rendered it: React keeps each eye's
// resting path in data-rest, current for the shape and mood it now shows.
function rest(face: LiveFace) {
  for (const part of [face.body, ...face.eyes, face.highlight, face.shade]) {
    part?.removeAttribute('transform')
  }
  face.eyes.forEach((eye) => {
    eye.setAttribute('d', eye.dataset.rest ?? '')
  })
  for (const extra of [face.ring, ...face.dots, ...face.zs]) {
    extra.setAttribute('opacity', '0')
  }
}

const faces = new Map<Element, LiveFace>()
const pointer = { x: 0, y: 0, at: -Infinity }
let frame = 0
let observer: IntersectionObserver | null = null

const BOX_AGE = 250

function onPointer(event: PointerEvent) {
  pointer.x = event.clientX
  pointer.y = event.clientY
  pointer.at = performance.now()
}

function onLeave(event: PointerEvent) {
  if (!event.relatedTarget) {
    pointer.at = -Infinity
  }
}

// Eases the cursor direction and resets it while the idle head turn plays.
function aim(face: LiveFace, now: number, following: boolean) {
  let target: [number, number] = [0, 0]
  if (following) {
    if (!face.box || now - face.box.at > BOX_AGE) {
      face.box = { rect: face.svg.getBoundingClientRect(), at: now }
    }
    const { rect } = face.box
    target = [
      Math.max(-1, Math.min(1, (pointer.x - rect.left - rect.width / 2) / 160)),
      Math.max(-1, Math.min(1, (pointer.y - rect.top - rect.height / 2) / 160)),
    ]
  }
  face.look = [face.look[0] + (target[0] - face.look[0]) * 0.12, face.look[1] + (target[1] - face.look[1]) * 0.12]
}

function tick(now: number) {
  frame = 0
  const time = now / 1000
  const recent = now - pointer.at < 2500
  // Every face is measured before any face is written.
  const shown = [...faces.values()].filter((face) => face.visible)
  for (const face of shown) {
    aim(face, now, face.follow && recent)
  }
  for (const face of shown) {
    const following = face.follow && recent
    const clip = face.clip
    const at = clip ? time - clip.start : 0
    if (clip && at >= CLIP_LENGTH[clip.name]) {
      face.clip = null
    }
    apply(face, face.clip ? clipPose(face, face.clip.name, at, time) : moodPose(face, time, following))
  }
  schedule()
}

function schedule() {
  if (!frame && !document.hidden && [...faces.values()].some((face) => face.visible)) {
    frame = requestAnimationFrame(tick)
  }
}

function watch(face: LiveFace) {
  if (!observer) {
    observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        const seen = faces.get(entry.target)
        if (seen) {
          seen.visible = entry.isIntersecting
          seen.box = null
        }
      }
      schedule()
    })
  }
  observer.observe(face.svg)
}

// Brings a face to life until stopped; the SVG marks its moving parts with
// data-face: body, eye, highlight, shade, ring, dot and z.
export function liveFace(svg: SVGSVGElement, geometry: FaceGeometry, mood: BotMood, size: number) {
  const part = <T extends Element>(name: string) => svg.querySelector<T>(`[data-face="${name}"]`)
  const parts = <T extends Element>(name: string) => [...svg.querySelectorAll<T>(`[data-face="${name}"]`)]
  const face: LiveFace = {
    svg,
    body: part<SVGGElement>('body')!,
    eyes: parts<SVGPathElement>('eye'),
    highlight: part<SVGElement>('highlight'),
    shade: part<SVGElement>('shade'),
    ring: part<SVGCircleElement>('ring')!,
    dots: parts<SVGCircleElement>('dot'),
    zs: parts<SVGTextElement>('z'),
    geometry,
    follow: size >= FOLLOW_SIZE,
    seed: Math.random(),
    mood,
    since: performance.now() / 1000,
    clip: null,
    visible: false,
    box: null,
    look: [0, 0],
    drawn: restingEyes(mood),
  }
  if (!faces.size) {
    window.addEventListener('pointermove', onPointer, { passive: true })
    document.addEventListener('pointerout', onLeave)
    document.addEventListener('visibilitychange', schedule)
  }
  faces.set(svg, face)
  watch(face)
  return {
    setMood(next: BotMood) {
      if (next === face.mood) {
        return
      }
      const now = performance.now() / 1000
      const clip = clipFor(face.mood, next)
      face.clip = clip ? { name: clip, start: now } : null
      face.mood = next
      face.since = now + (clip ? CLIP_LENGTH[clip] : 0)
    },
    celebrate() {
      const now = performance.now() / 1000
      face.clip = { name: 'cheer', start: now }
      face.since = now + CLIP_LENGTH.cheer
    },
    stop() {
      faces.delete(svg)
      observer?.unobserve(svg)
      rest(face)
      if (!faces.size) {
        window.removeEventListener('pointermove', onPointer)
        document.removeEventListener('pointerout', onLeave)
        document.removeEventListener('visibilitychange', schedule)
        cancelAnimationFrame(frame)
        frame = 0
      }
    },
  }
}
