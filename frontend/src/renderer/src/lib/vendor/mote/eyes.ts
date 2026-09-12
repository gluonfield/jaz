import { closedCurvePath, type Point } from '@/lib/vendor/mote/shapes'

export const EYE_IDS = [
  'neutral',
  'focused',
  'joyful',
  'surprised',
  'sleepy',
  'skeptical',
  'dots',
  'determined',
  'soft',
  'scanning',
  'listening',
  'happy',
  'wide',
  'closed',
  'side-eye',
  'proud',
  'shy',
  'thinking',
  'searching',
  'attentive',
  'uneasy',
  'wonder',
  'drowsy',
  'suspicious',
  'spark',
] as const

export type EyeId = (typeof EYE_IDS)[number]

export type EyeDefinition = {
  id: EyeId
  referenceIndex: number
  label: string
  description: string
  leftPath: string
  rightPath: string
  leftCenter: { x: number; y: number }
  rightCenter: { x: number; y: number }
}

type EyeGeometry = {
  cx: number
  cy: number
  rx: number
  ry: number
  rotation?: number
  power?: number
}

const eyeGeometry = (
  cx: number,
  cy: number,
  rx: number,
  ry: number,
  rotation = 0,
  power = 3,
): EyeGeometry => ({ cx, cy, rx, ry, rotation, power })

const eyePath = ({
  cx,
  cy,
  rx,
  ry,
  rotation = 0,
  power = 3,
}: EyeGeometry): string => {
  const radians = (rotation * Math.PI) / 180
  const cosine = Math.cos(radians)
  const sine = Math.sin(radians)
  const points: Point[] = Array.from({ length: 16 }, (_, index) => {
    const angle = -Math.PI / 2 + (index / 16) * Math.PI * 2
    const localX =
      rx * Math.sign(Math.cos(angle)) * Math.abs(Math.cos(angle)) ** (2 / power)
    const localY =
      ry * Math.sign(Math.sin(angle)) * Math.abs(Math.sin(angle)) ** (2 / power)

    return {
      x: cx + localX * cosine - localY * sine,
      y: cy + localX * sine + localY * cosine,
    }
  })

  return closedCurvePath(points)
}

const specs: Record<EyeId, readonly [EyeGeometry, EyeGeometry]> = {
  neutral: [eyeGeometry(134, 153, 12, 28, 8), eyeGeometry(189, 153, 12, 28, 8)],
  focused: [eyeGeometry(136, 153, 9, 23, 4), eyeGeometry(187, 153, 9, 23, 4)],
  joyful: [eyeGeometry(132, 154, 17, 25, -8), eyeGeometry(191, 154, 17, 25, 8)],
  surprised: [
    eyeGeometry(132, 151, 19, 24, 0, 2.2),
    eyeGeometry(192, 151, 19, 24, 0, 2.2),
  ],
  sleepy: [eyeGeometry(133, 159, 22, 5, -3), eyeGeometry(190, 159, 22, 5, 3)],
  skeptical: [
    eyeGeometry(134, 153, 19, 6, -12),
    eyeGeometry(189, 153, 10, 24, 8),
  ],
  dots: [
    eyeGeometry(135, 154, 8, 8, 0, 2.2),
    eyeGeometry(188, 154, 8, 8, 0, 2.2),
  ],
  determined: [
    eyeGeometry(134, 153, 12, 27, -17),
    eyeGeometry(189, 153, 12, 27, 17),
  ],
  soft: [eyeGeometry(134, 157, 14, 23, 5), eyeGeometry(189, 157, 14, 23, -5)],
  scanning: [
    eyeGeometry(141, 153, 10, 25, 4),
    eyeGeometry(196, 153, 10, 25, 4),
  ],
  listening: [
    eyeGeometry(133, 150, 11, 29, 6),
    eyeGeometry(190, 157, 10, 22, 6),
  ],
  happy: [eyeGeometry(133, 157, 19, 7, 10), eyeGeometry(190, 157, 19, 7, -10)],
  wide: [eyeGeometry(131, 151, 20, 32, 5), eyeGeometry(193, 151, 20, 32, -5)],
  closed: [eyeGeometry(133, 158, 23, 3, 0), eyeGeometry(190, 158, 23, 3, 0)],
  'side-eye': [
    eyeGeometry(128, 153, 10, 24, 7),
    eyeGeometry(181, 153, 10, 24, 7),
  ],
  proud: [eyeGeometry(133, 154, 18, 7, -9), eyeGeometry(190, 154, 18, 7, 9)],
  shy: [eyeGeometry(136, 166, 9, 17, 10), eyeGeometry(187, 166, 9, 17, -10)],
  thinking: [
    eyeGeometry(132, 145, 8, 14, -8),
    eyeGeometry(190, 155, 13, 27, 10),
  ],
  searching: [
    eyeGeometry(128, 152, 12, 27, 12),
    eyeGeometry(183, 152, 12, 27, 12),
  ],
  attentive: [
    eyeGeometry(132, 151, 14, 32, 4),
    eyeGeometry(192, 151, 14, 32, -4),
  ],
  uneasy: [
    eyeGeometry(134, 155, 12, 25, -13),
    eyeGeometry(189, 155, 12, 25, -13),
  ],
  wonder: [eyeGeometry(131, 150, 20, 30, -4), eyeGeometry(193, 156, 10, 18, 8)],
  drowsy: [eyeGeometry(134, 161, 20, 6, 5), eyeGeometry(189, 161, 20, 6, -5)],
  suspicious: [
    eyeGeometry(133, 153, 19, 6, 12),
    eyeGeometry(190, 153, 14, 18, -11),
  ],
  spark: [
    eyeGeometry(131, 151, 20, 31, -11, 2.3),
    eyeGeometry(193, 151, 20, 31, 11, 2.3),
  ],
}

const metadata: Record<EyeId, { label: string; description: string }> = {
  neutral: { label: 'Neutral', description: 'Balanced and available' },
  focused: { label: 'Focused', description: 'Small and concentrated' },
  joyful: { label: 'Joyful', description: 'Open and buoyant' },
  surprised: { label: 'Surprised', description: 'Round and alert' },
  sleepy: { label: 'Sleepy', description: 'Nearly closed' },
  skeptical: { label: 'Skeptical', description: 'Deliberately asymmetric' },
  dots: { label: 'Dots', description: 'Tiny and computational' },
  determined: { label: 'Determined', description: 'Angled toward the center' },
  soft: { label: 'Soft', description: 'Gentle and relaxed' },
  scanning: { label: 'Scanning', description: 'Shifted toward a target' },
  listening: { label: 'Listening', description: 'Uneven and attentive' },
  happy: { label: 'Happy', description: 'Closed with a smile' },
  wide: { label: 'Wide', description: 'Large and expressive' },
  closed: { label: 'Closed', description: 'A quiet resting line' },
  'side-eye': { label: 'Side-eye', description: 'Looking off-axis' },
  proud: { label: 'Proud', description: 'Lifted at the edges' },
  shy: { label: 'Shy', description: 'Small and lowered' },
  thinking: { label: 'Thinking', description: 'Curious asymmetry' },
  searching: { label: 'Searching', description: 'Tracking the horizon' },
  attentive: { label: 'Attentive', description: 'Tall and present' },
  uneasy: { label: 'Uneasy', description: 'A shared diagonal' },
  wonder: { label: 'Wonder', description: 'One question, one answer' },
  drowsy: { label: 'Drowsy', description: 'Heavy and low' },
  suspicious: { label: 'Suspicious', description: 'Narrowed and uneven' },
  spark: { label: 'Spark', description: 'Bright and emphatic' },
}

export const EYES: EyeDefinition[] = EYE_IDS.map((id, referenceIndex) => ({
  id,
  referenceIndex,
  ...metadata[id],
  leftPath: eyePath(specs[id][0]),
  rightPath: eyePath(specs[id][1]),
  leftCenter: { x: specs[id][0].cx, y: specs[id][0].cy },
  rightCenter: { x: specs[id][1].cx, y: specs[id][1].cy },
}))

export const eyeById = (id: EyeId): EyeDefinition => {
  const eye = EYES.find((candidate) => candidate.id === id)
  if (!eye) throw new Error(`Unknown Mote eye expression: ${id}`)
  return eye
}
