import type { BotMood } from '@/lib/bots'

// One animation clock for every live bot face on screen. Each frame works out
// a pose per face and writes it straight into its SVG, so motion never renders
// React. Faces off screen, in a hidden window or under reduced motion hold
// still.

export type EyeKind = 'open' | 'happy' | 'worried' | 'sleepy'

// Where a face's parts sit on its 100×100 grid.
export type FaceGeometry = { eyeLine: number; bottom: number }

// The eyes' centres; drawn right of centre, so the face glances aside.
const EYE_X = [47, 67] as const

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
  return mood === 'away' ? 'sleepy' : 'open'
}

// A pose in face units. `yaw` turns the head (-1 to 1): the eyes slide across
// a sphere and narrow toward its edge while the light stays put, which is what
// reads as depth.
type Pose = {
  x: number
  y: number
  sx: number
  sy: number
  lookX: number
  lookY: number
  open: number
  yaw: number
  eyes: EyeKind
}

function still(eyes: EyeKind): Pose {
  return { x: 0, y: 0, sx: 1, sy: 1, lookX: 0, lookY: 0, open: 1, yaw: 0, eyes }
}

type Clip = 'nod' | 'cheer' | 'shake'

const CLIP_LENGTH: Record<Clip, number> = { nod: 0.6, cheer: 1.4, shake: 0.7 }

// The move a change of mood plays once before the new mood's loop. A failed
// bot shakes once and then rests; its badge says the rest.
function clipFor(from: BotMood, to: BotMood): Clip | null {
  if (to === 'failed') {
    return 'shake'
  }
  if (to === 'working' && from !== 'working') {
    return 'nod'
  }
  if (from === 'working' && to === 'idle') {
    return 'cheer'
  }
  return null
}

type LiveFace = {
  svg: SVGSVGElement
  body: SVGGElement
  eyes: SVGPathElement[]
  geometry: FaceGeometry
  seed: number
  mood: BotMood
  since: number
  clip: { name: Clip; start: number } | null
  visible: boolean
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
  const period = 3.4 + seed * 1.6
  const local = (time + seed * 7) % period
  const cycle = Math.floor((time + seed * 7) / period)
  const first = bump(local, 0, 0.18)
  return cycle % 3 === 2 ? Math.max(first, bump(local, 0.26, 0.44)) : first
}

// Now and then the face turns a little one way, holds, the other way, holds
// and comes back; most of the time it rests facing ahead.
// A working face's eyes move between what it works on: a look one way, then
// the other.
const GLANCE: [number, number][] = [[0, 0], [1.2, 0], [1.4, 3], [2.4, 3], [2.6, -2], [3.4, -2], [3.6, 0], [4.4, 0]]

const WANDER: [number, number][] = [[0, 0], [5, 0], [6.2, 0.5], [8.5, 0.5], [9.7, -0.45], [12, -0.45], [13.2, 0], [16, 0]]

function moodPose(face: LiveFace, time: number): Pose {
  const local = time - face.since
  const pose = still(restingEyes(face.mood))
  const breath = Math.sin(((time + face.seed * 5) * Math.PI) / 2)
  pose.sy = 1 + 0.025 * breath
  pose.sx = 1 - 0.015 * breath
  pose.open = 1 - 0.9 * blink(time, face.seed)
  pose.yaw = track(WANDER, time + face.seed * 11)
  pose.lookY = Math.sin(time * 0.4 + face.seed * 5) * 0.8
  if (face.mood === 'working') {
    pose.y = -2.5 * Math.abs(Math.sin((local * Math.PI) / 1.6))
    pose.lookX = track(GLANCE, local)
    pose.yaw *= 0.3
    pose.open *= 0.85
  } else if (face.mood === 'waiting') {
    const lean = 0.03 * bump(local % 2.4, 0, 0.8)
    pose.sx = 1 + lean
    pose.sy = 1 + lean
    pose.yaw = 0
    pose.lookX = -7
    pose.lookY = 0
    pose.open *= 1.12
  } else if (face.mood === 'away') {
    pose.y = Math.sin(time * 0.9) * 1.2
    pose.sy = 1 + 0.015 * Math.sin(time * 0.9)
    pose.open = 1
    pose.yaw *= 0.2
  }
  return pose
}

function clipPose(face: LiveFace, clip: Clip, at: number, time: number): Pose {
  const pose = moodPose(face, time)
  if (clip === 'nod') {
    pose.sy *= 1 - 0.07 * bump(at, 0, 0.14)
    pose.y += 3 * bump(at, 0.1, 0.45)
    pose.lookY += 4 * bump(at, 0.1, 0.45)
    pose.open = 1 + 0.25 * bump(at, 0.35, 0.6)
  } else if (clip === 'cheer') {
    const squash = bump(at, 0, 0.3)
    pose.eyes = 'happy'
    pose.open = 1
    pose.sy = 1 - 0.05 * squash
    pose.sx = 1 + 0.035 * squash
  } else {
    pose.eyes = 'worried'
    pose.x = Math.sin(at * 42) * 3.5 * (1 - at / CLIP_LENGTH.shake)
  }
  return pose
}

function apply(face: LiveFace, pose: Pose) {
  const { eyeLine, bottom } = face.geometry
  face.body.setAttribute(
    'transform',
    `translate(${pose.x} ${pose.y}) translate(50 ${bottom}) scale(${pose.sx} ${pose.sy}) translate(-50 ${-bottom})`,
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
    const narrow = Math.max(0.3, Math.cos(turned) / Math.cos(angle))
    eye.setAttribute(
      'transform',
      `translate(${cx + slide + pose.lookX} ${eyeLine + pose.lookY}) scale(${narrow} ${pose.open}) translate(${-cx} ${-eyeLine})`,
    )
  })
}

// Hands the face back as React last rendered it: React keeps each eye's
// resting path in data-rest, current for the shape and mood it now shows.
function rest(face: LiveFace) {
  face.body.removeAttribute('transform')
  face.eyes.forEach((eye) => {
    eye.removeAttribute('transform')
    eye.setAttribute('d', eye.dataset.rest ?? '')
  })
}

const faces = new Map<Element, LiveFace>()
let frame = 0
let observer: IntersectionObserver | null = null

function tick(now: number) {
  frame = 0
  const time = now / 1000
  for (const face of faces.values()) {
    if (!face.visible) {
      continue
    }
    const clip = face.clip
    const at = clip ? time - clip.start : 0
    if (clip && at >= CLIP_LENGTH[clip.name]) {
      face.clip = null
    }
    apply(face, face.clip ? clipPose(face, face.clip.name, at, time) : moodPose(face, time))
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
        }
      }
      schedule()
    })
  }
  observer.observe(face.svg)
}

// Brings a face to life until stopped; the SVG marks its moving parts with
// data-face="body" and "eye".
export function liveFace(svg: SVGSVGElement, geometry: FaceGeometry, mood: BotMood) {
  const body = svg.querySelector<SVGGElement>('[data-face="body"]')!
  const face: LiveFace = {
    svg,
    body,
    eyes: [...svg.querySelectorAll<SVGPathElement>('[data-face="eye"]')],
    geometry,
    seed: Math.random(),
    mood,
    since: performance.now() / 1000,
    clip: null,
    visible: false,
    drawn: restingEyes(mood),
  }
  if (!faces.size) {
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
      face.since = now
    },
    stop() {
      faces.delete(svg)
      observer?.unobserve(svg)
      rest(face)
      if (!faces.size) {
        document.removeEventListener('visibilitychange', schedule)
        cancelAnimationFrame(frame)
        frame = 0
      }
    },
  }
}
