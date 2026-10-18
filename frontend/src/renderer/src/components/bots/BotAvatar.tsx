import { Star } from 'lucide-react'
import { motion } from 'motion/react'
import type { ReactNode, SVGProps } from 'react'
import type { BotAvatar as Avatar, BotShape } from '@/lib/api/types'
import { BOT_COLORS } from '@/lib/bots'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'

// Drawn on a 100×100 grid; `eyes` is the vertical center of the eye pair.
// The eyes are tilted capsules set right of center, so the face glances aside.
// Triangle and hex round their corners with a same-color stroke.
const SHAPES: Record<BotShape, { eyes: number; body: (fill: string) => ReactNode }> = {
  circle: { eyes: 46, body: () => <circle cx="50" cy="50" r="46" /> },
  blob: {
    eyes: 46,
    body: () => <path d="M56 5c22 2 38 18 38 40 0 16-6 25-8 36-3 13-19 16-34 14C27 92 6 80 6 55 6 26 29 3 56 5z" />,
  },
  squircle: { eyes: 46, body: () => <rect x="6" y="6" width="88" height="88" rx="32" /> },
  pill: { eyes: 42, body: () => <rect x="20" y="4" width="60" height="92" rx="30" /> },
  triangle: {
    eyes: 58,
    body: (fill) => <path d="M50 14 88 80H12z" stroke={fill} strokeWidth="16" strokeLinejoin="round" />,
  },
  hex: {
    eyes: 46,
    body: (fill) => <path d="m50 11 34 19.5v39L50 89 16 69.5v-39z" stroke={fill} strokeWidth="12" strokeLinejoin="round" />,
  },
  cloud: {
    eyes: 54,
    body: () => (
      <>
        <circle cx="32" cy="60" r="22" />
        <circle cx="56" cy="44" r="28" />
        <circle cx="74" cy="62" r="20" />
        <rect x="12" y="56" width="76" height="26" rx="13" />
      </>
    ),
  },
  drop: { eyes: 60, body: () => <path d="M50 4s36 38 36 59a36 36 0 0 1-72 0C14 42 50 4 50 4z" /> },
}

// A working face bobs and glances around, unless motion is reduced.
const LOOP = { repeat: Infinity, ease: 'easeInOut' } as const

export function BotAvatar({
  avatar,
  size,
  working = false,
  className = '',
}: {
  avatar: Avatar
  size: number
  working?: boolean
  className?: string
}) {
  const fill = BOT_COLORS[avatar.color]
  const { eyes, body } = SHAPES[avatar.shape]
  const reduced = useReducedEffectsMotion()
  const animate = working && !reduced
  return (
    <motion.svg
      aria-hidden
      viewBox="0 0 100 100"
      width={size}
      height={size}
      animate={{ y: animate ? [0, -size * 0.06, 0] : 0 }}
      transition={animate ? { duration: 1.2, ...LOOP } : undefined}
      className={`shrink-0 overflow-visible drop-shadow-[0_0_0.5px_rgb(0_0_0/0.35)] ${className}`}
    >
      <g fill={fill}>{body(fill)}</g>
      <motion.path
        d={`M45 ${eyes - 7}l4 14M65 ${eyes - 7}l4 14`}
        stroke="#1c1c1f"
        strokeWidth="7"
        strokeLinecap="round"
        animate={{ x: animate ? [0, 5, 5, -5, -5, 0] : 0 }}
        transition={animate ? { duration: 2.8, times: [0, 0.15, 0.4, 0.55, 0.8, 1], ...LOOP } : undefined}
      />
    </motion.svg>
  )
}

// One face for a bot; a group overlaps its first two members. A working bot
// wears a green dot, like someone online; otherwise `badge` may mark it. The
// main bot wears a star at the bottom right.
export function BotIcon({ avatars, size, working = false, badge, main = false }: {
  avatars: Avatar[]
  size: number
  working?: boolean
  badge?: { title: string; className: string } | null
  main?: boolean
}) {
  const member = Math.round(size * 0.72)
  const dot = working ? { title: 'Working', className: 'bg-ok' } : badge
  return (
    <span className="relative shrink-0" style={{ width: size, height: size }}>
      {avatars.length < 2 ? (
        <BotAvatar avatar={avatars[0]} size={size} working={working} />
      ) : (
        <>
          <BotAvatar avatar={avatars[0]} size={member} className="absolute left-0 top-0" />
          <BotAvatar avatar={avatars[1]} size={member} className="absolute right-0 bottom-0" />
        </>
      )}
      {dot ? (
        <span
          title={dot.title}
          className={`absolute -top-px -right-px rounded-full ring-2 ring-panel ${dot.className}`}
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
