import { Star } from 'lucide-react'
import { type ReactNode, type SVGProps, useEffect, useId, useRef } from 'react'
import type { BotAvatar as Avatar, BotShape } from '@/lib/api/types'
import { BOT_COLORS, type BotMood } from '@/lib/bots'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { eyePath, type FaceGeometry, liveFace, restingEyes } from './faceMotion'

// Drawn on a 100×100 grid; `eyeLine` is the vertical center of the eye pair
// and `bottom` where the body meets the ground, which squashes about it.
// The eyes are tilted capsules set right of center, so the face glances aside.
// Triangle and hex round their corners with a same-color stroke.
const SHAPES: Record<BotShape, FaceGeometry & { body: (fill: string) => ReactNode }> = {
  circle: { eyeLine: 46, bottom: 96, body: () => <circle cx="50" cy="50" r="46" /> },
  blob: {
    eyeLine: 46,
    bottom: 95,
    body: () => <path d="M56 5c22 2 38 18 38 40 0 16-6 25-8 36-3 13-19 16-34 14C27 92 6 80 6 55 6 26 29 3 56 5z" />,
  },
  squircle: { eyeLine: 46, bottom: 94, body: () => <rect x="6" y="6" width="88" height="88" rx="32" /> },
  pill: { eyeLine: 42, bottom: 96, body: () => <rect x="20" y="4" width="60" height="92" rx="30" /> },
  triangle: {
    eyeLine: 58,
    bottom: 88,
    body: (fill) => <path d="M50 14 88 80H12z" stroke={fill} strokeWidth="16" strokeLinejoin="round" />,
  },
  hex: {
    eyeLine: 46,
    bottom: 95,
    body: (fill) => <path d="m50 11 34 19.5v39L50 89 16 69.5v-39z" stroke={fill} strokeWidth="12" strokeLinejoin="round" />,
  },
  cloud: {
    eyeLine: 54,
    bottom: 82,
    body: () => (
      <>
        <circle cx="32" cy="60" r="22" />
        <circle cx="56" cy="44" r="28" />
        <circle cx="74" cy="62" r="20" />
        <rect x="12" y="56" width="76" height="26" rx="13" />
      </>
    ),
  },
  drop: { eyeLine: 60, bottom: 99, body: () => <path d="M50 4s36 38 36 59a36 36 0 0 1-72 0C14 42 50 4 50 4z" /> },
}

// A bot's shape alone, in its colour, for use as a backdrop.
export function BotSilhouette({ avatar, size }: { avatar: Avatar; size: number }) {
  const fill = BOT_COLORS[avatar.color]
  return (
    <svg aria-hidden viewBox="0 0 100 100" width={size} height={size} className="shrink-0 overflow-visible">
      <g fill={fill}>{SHAPES[avatar.shape].body(fill)}</g>
    </svg>
  )
}

// A face shows its bot's mood. A live one moves gently (breathes, blinks,
// now and then looks around, and acts out the mood), unless motion is
// reduced; any other face holds still. Faces large
// enough to read it carry a soft light from the top left, which the moving
// eyes turn under.
export function BotAvatar({
  avatar,
  size,
  mood = 'idle',
  live = false,
  className = '',
}: {
  avatar: Avatar
  size: number
  mood?: BotMood
  live?: boolean
  className?: string
}) {
  const fill = BOT_COLORS[avatar.color]
  const shape = SHAPES[avatar.shape]
  const svg = useRef<SVGSVGElement>(null)
  const face = useRef<ReturnType<typeof liveFace> | null>(null)
  const reduced = useReducedEffectsMotion()
  const moving = live && !reduced
  const light = `face-light-${useId().replace(/[^\w-]/g, '')}`
  useEffect(() => {
    if (!moving || !svg.current) {
      return
    }
    face.current = liveFace(svg.current, shape, mood)
    return () => {
      face.current?.stop()
      face.current = null
    }
    // A new face starts from the mood it has; later moods only change it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [moving, shape])
  useEffect(() => {
    face.current?.setMood(mood)
  }, [mood])
  const eyes = restingEyes(mood)
  return (
    <svg
      ref={svg}
      aria-hidden
      viewBox="0 0 100 100"
      width={size}
      height={size}
      className={`shrink-0 overflow-visible drop-shadow-[0_0_0.5px_rgb(0_0_0/0.35)] ${className}`}
    >
      <g data-face="body">
        <g fill={fill}>{shape.body(fill)}</g>
        {size >= 24 ? (
          <>
            <radialGradient id={light} gradientUnits="userSpaceOnUse" cx="34" cy="26" r="82">
              <stop offset="0" stopColor="#fff" stopOpacity="0.32" />
              <stop offset="0.42" stopColor="#fff" stopOpacity="0" />
              <stop offset="1" stopColor="#000" stopOpacity="0.16" />
            </radialGradient>
            <mask id={`${light}-shape`}>
              <g fill="#fff">{shape.body('#fff')}</g>
            </mask>
            <rect width="100" height="100" fill={`url(#${light})`} mask={`url(#${light}-shape)`} />
          </>
        ) : null}
        <g stroke="#1c1c1f" strokeWidth="7" strokeLinecap="round" fill="none">
          {[47, 67].map((x) => {
            const d = eyePath(eyes, x, shape.eyeLine)
            return <path key={x} data-face="eye" data-rest={d} d={d} />
          })}
        </g>
      </g>
    </svg>
  )
}

// One face for a bot; a group overlaps its first two members. `badge` may
// mark it, and the main bot wears a star at the bottom right.
export function BotIcon({ avatars, size, mood, live = false, badge, main = false }: {
  avatars: Avatar[]
  size: number
  mood?: BotMood
  live?: boolean
  badge?: { title: string; className: string } | null
  main?: boolean
}) {
  const member = Math.round(size * 0.72)
  return (
    <span className="relative shrink-0" style={{ width: size, height: size }}>
      {avatars.length < 2 ? (
        <BotAvatar avatar={avatars[0]} size={size} mood={mood} live={live} />
      ) : (
        <>
          <BotAvatar avatar={avatars[0]} size={member} mood={mood} live={live} className="absolute left-0 top-0" />
          <BotAvatar avatar={avatars[1]} size={member} mood={mood} live={live} className="absolute right-0 bottom-0" />
        </>
      )}
      {badge ? (
        <span
          role="img"
          aria-label={badge.title}
          title={badge.title}
          className={`absolute -top-px -right-px rounded-full ring-2 ring-panel ${badge.className}`}
          style={{ width: Math.max(7, Math.round(size * 0.24)), height: Math.max(7, Math.round(size * 0.24)) }}
        />
      ) : null}
      {main ? (
        <span
          title="Main bot"
          className="absolute -right-0.5 -bottom-0.5 grid place-items-center rounded-full bg-accent text-white ring-2 ring-panel"
          style={{ width: Math.round(size * 0.32), height: Math.round(size * 0.32) }}
        >
          <Star size={Math.round(size * 0.19)} fill="currentColor" strokeWidth={0} aria-hidden />
        </span>
      ) : null}
    </span>
  )
}

// The bot's identity centered in the titlebar.
export function BotPill({ avatars, name }: { avatars: Avatar[]; name: string }) {
  return (
    <span className="mx-auto flex min-w-0 items-center gap-1.5 rounded-full bg-surface py-0.5 pr-3 pl-1 text-[13px] font-medium text-ink">
      <BotIcon avatars={avatars} size={22} />
      <span className="truncate">{name}</span>
    </span>
  )
}

// The Bots section icon: an outline face in the icon set's stroke style.
export function BotsGlyph(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="24"
      height="24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <rect x="3" y="3" width="18" height="18" rx="7" />
      <path d="M11 9.3l.8 3M14.8 9.3l.8 3" strokeWidth="2.2" />
    </svg>
  )
}
