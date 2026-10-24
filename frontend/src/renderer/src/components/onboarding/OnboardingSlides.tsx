import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Check, FileText, type LucideIcon, Mail } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useState } from 'react'
import type { SystemPermission } from '@shared/systemPermissions'
import { BotAvatar } from '@/components/bots/BotAvatar'
import { DitherWordmark, type Silhouette } from '@/components/launch/DitherArt'
import { PresetSwatches } from '@/components/settings/ColorThemeSettings'
import { ThemeModePicker } from '@/components/settings/ThemeModePicker'
import { Button } from '@/components/ui/Button'
import { botsQuery } from '@/lib/api/bots'
import type { BotAvatar as BotAvatarShape } from '@/lib/api/types'
import { BOT_COLORS } from '@/lib/bots'
import { useReducedEffectsMotion } from '@/lib/effectsMotion'
import { type SetupStep, onboardingEase, slideExit } from './OnboardingParts'

// The first thing a new user ever sees: the wordmark assembles itself out of
// dither grain, then one line of copy and the CTA rise in underneath it.
export function WelcomeStep({ onStart }: { onStart: () => void }) {
  return (
    <motion.div exit={slideExit} className="flex w-full max-w-[440px] flex-col items-center text-center">
      <DitherWordmark delay={0.25} />
      <motion.p
        initial={{ opacity: 0, y: 10, filter: 'blur(4px)' }}
        animate={{ opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.5, delay: 1.05, ease: onboardingEase } }}
        className="mt-9 text-pretty text-[15px] leading-relaxed text-ink-2"
      >
        Agents that code, remember you, and keep working while you're away.
      </motion.p>
      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0, transition: { duration: 0.4, delay: 1.3, ease: onboardingEase } }}
        className="mt-8"
      >
        <Button variant="primary" size="lg" onClick={onStart} className="h-11 px-6 text-[14px]">
          Get started
          <ArrowRight size={15} />
        </Button>
      </motion.div>
    </motion.div>
  )
}

// A soft radial gradient behind each mark: the dither turns it into a grain
// halo, which is what gives the slide art its shading depth.
function heroGlow(g: Parameters<Silhouette>[0], w: number, h: number) {
  const glow = g.createRadialGradient(w * 0.5, h * 0.5, 0, w * 0.5, h * 0.5, h * 0.95)
  glow.addColorStop(0, 'rgba(255,255,255,0.55)')
  glow.addColorStop(1, 'rgba(255,255,255,0)')
  g.fillStyle = glow
  g.fillRect(0, 0, w, h)
  g.fillStyle = '#fff'
}

// The setup slides in order; the last one finishes setup.
export const SETUP_STEPS: SetupStep[] = ['appearance', 'agents', 'memory', 'connections', 'computer', 'voice', 'bot']

// Everything that defines a setup slide lives in this one table: the dithered
// hero mark, the copy, and the system permissions it asks for. The bot slide
// has no mark: its bots are the picture.
export const SLIDES: Record<SetupStep, { motif?: Silhouette; title: string; subtitle: string; permissions?: SystemPermission[] }> = {
  appearance: {
    title: 'Make it yours',
    subtitle: 'Pick a mode and a color. Fine-tune both any time in Settings.',
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      const disc = new Path2D()
      disc.arc(w * 0.5, h * 0.5, h * 0.32, 0, Math.PI * 2)
      // The glow is cut from the disc so its empty half reads as empty.
      g.globalCompositeOperation = 'destination-out'
      g.fill(disc)
      g.globalCompositeOperation = 'source-over'
      g.lineWidth = h * 0.1
      g.stroke(disc)
      g.beginPath()
      g.arc(w * 0.5, h * 0.5, h * 0.32, -Math.PI / 2, Math.PI / 2)
      g.fill()
    },
  },
  agents: {
    title: 'Connect your agents',
    subtitle: 'jaz runs on the coding agents you already use.',
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      g.lineWidth = h * 0.16
      g.lineCap = 'round'
      g.lineJoin = 'round'
      g.beginPath()
      g.moveTo(w * 0.41, h * 0.24)
      g.lineTo(w * 0.52, h * 0.5)
      g.lineTo(w * 0.41, h * 0.76)
      g.stroke()
      g.beginPath()
      g.roundRect(w * 0.56, h * 0.66, w * 0.075, h * 0.12, h * 0.05)
      g.fill()
    },
  },
  memory: {
    title: 'Give jaz a memory',
    subtitle: 'Preferences, decisions, projects — agents start warm, not cold.',
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      for (const [y, half] of [
        [0.16, 0.1],
        [0.44, 0.14],
        [0.72, 0.18],
      ] as const) {
        g.beginPath()
        g.roundRect(w * (0.5 - half), h * y, w * half * 2, h * 0.15, h * 0.075)
        g.fill()
      }
    },
  },
  connections: {
    title: 'Connect your world',
    subtitle: 'Your email and chats become context agents can use.',
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      g.lineWidth = h * 0.12
      for (const x of [0.43, 0.57]) {
        g.beginPath()
        g.arc(w * x, h * 0.5, h * 0.3, 0, Math.PI * 2)
        g.stroke()
      }
    },
  },
  computer: {
    title: 'Let jaz use your computer',
    subtitle: 'Agents can click, type and read apps to finish tasks while you are away.',
    permissions: ['accessibility', 'screenRecording'],
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      g.lineWidth = h * 0.08
      g.beginPath()
      g.roundRect(w * 0.36, h * 0.12, w * 0.25, h * 0.62, h * 0.08)
      g.stroke()
      const x = w * 0.53
      const y = h * 0.4
      const s = h * 0.56
      const cursor = new Path2D()
      cursor.moveTo(x, y)
      cursor.lineTo(x, y + s)
      cursor.lineTo(x + s * 0.3, y + s * 0.76)
      cursor.lineTo(x + s * 0.66, y + s * 0.76)
      cursor.closePath()
      // A cut-out halo keeps the cursor legible where it overlaps the window.
      g.globalCompositeOperation = 'destination-out'
      g.lineWidth = h * 0.18
      g.stroke(cursor)
      g.globalCompositeOperation = 'source-over'
      g.fill(cursor)
    },
  },
  voice: {
    title: 'Talk to jaz',
    subtitle: 'Speak instead of typing, and get notified when a thread finishes.',
    permissions: ['microphone', 'notifications'],
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      const bars = [0.25, 0.5, 0.8, 0.55, 1, 0.6, 0.85, 0.45, 0.3]
      const step = w * 0.05
      bars.forEach((size, index) => {
        const height = h * 0.76 * size
        g.beginPath()
        g.roundRect(w * 0.5 + (index - (bars.length - 1) / 2) * step - step * 0.25, (h - height) / 2, step * 0.5, height, step * 0.25)
        g.fill()
      })
    },
  },
  bot: {
    title: 'Meet your bots',
    subtitle: "Each has a job, remembers everything you tell it, and keeps working while you're away.",
  },
}

// The two footer bits that depend on live state rather than the table.
export function slideFooter(
  step: SetupStep,
  state: { canContinue: boolean; memoryReady: boolean; skippable: boolean },
): { nextLabel: string; nextDisabled: boolean } {
  return {
    nextLabel: step === 'bot' ? 'Launch jaz' : state.skippable ? 'Skip for now' : 'Continue',
    nextDisabled: step === 'agents' ? !state.canContinue : step === 'memory' ? !state.memoryReady : false,
  }
}

// The mode tiles are miniature windows in the chosen colour, so a palette
// pick previews there and across the whole slide at once.
export function ThemeChoice() {
  return (
    <>
      <ThemeModePicker />
      <div className="mt-4 flex items-center justify-between gap-3 rounded-[14px] bg-surface p-3.5">
        <p className="text-[13.5px] font-medium text-ink">Color</p>
        <PresetSwatches />
      </div>
    </>
  )
}

// Three bots at work on an open stage: the main bot every install starts
// with, then two helpers the user could add. Each bot loops: typing dots, its
// task streaming in and running, then the result with a cheer. A helper's
// result travels its dotted line into the main bot, which catches it.
// Coordinates are in the stage's 800×340 space. Resting faces follow the
// cursor.
type TeamBot = {
  name?: string
  role: string
  avatar?: BotAvatarShape
  at: [number, number]
  size: number
  pace: number
  handoff?: { path: string; icon: LucideIcon }
  tasks: { doing: string; done: string }[]
}

const TEAM: TeamBot[] = [
  {
    role: 'Chief of staff',
    at: [400, 178],
    size: 96,
    pace: 12,
    tasks: [
      { doing: 'Booking lunch with Lena', done: 'Lunch with Lena, Thursday 12:30' },
      { doing: 'Planning your week', done: 'Mornings kept free for deep work' },
      { doing: 'Preparing your 1:1 notes', done: '1:1 notes ready' },
    ],
  },
  {
    name: 'Scout',
    role: 'Researcher',
    avatar: { shape: 'squircle', color: 'orange' },
    at: [140, 70],
    size: 64,
    pace: 13,
    handoff: { path: 'M 182 72 C 270 66, 330 96, 356 140', icon: FileText },
    tasks: [
      { doing: 'Reading 14 battery papers', done: 'Battery brief ready' },
      { doing: 'Comparing 3 CRM vendors', done: 'Vendor comparison ready' },
      { doing: 'Watching competitor launches', done: '2 launches flagged' },
    ],
  },
  {
    name: 'Relay',
    role: 'Inbox',
    avatar: { shape: 'pill', color: 'purple' },
    at: [660, 112],
    size: 68,
    pace: 8,
    handoff: { path: 'M 618 110 C 540 106, 474 120, 446 146', icon: Mail },
    tasks: [
      { doing: 'Sorting 48 new emails', done: '6 need you today' },
      { doing: 'Drafting replies', done: '4 drafts ready to send' },
      { doing: 'Chasing late invoices', done: 'Followed up with 3 clients' },
    ],
  },
]

// The loop runs on a 400 ms tick: two of typing, `pace` of work, five of
// result. A handoff lands four ticks after its result.
const TICK = 400
const TYPING = 2
const RESULT = 5
const FLIGHT = 4

function momentAt(bot: TeamBot, index: number, tick: number) {
  const local = tick - 2 - index
  const cycle = TYPING + bot.pace + RESULT
  const turn = Math.max(0, Math.floor(local / cycle))
  const phase = local - turn * cycle
  const count = (offset: number) => (local < offset ? 0 : Math.floor((local - offset) / cycle) + 1)
  return {
    stage: local < 0 ? 'hidden' : phase < TYPING ? 'typing' : phase < TYPING + bot.pace ? 'doing' : 'done',
    task: bot.tasks[turn % bot.tasks.length],
    turn,
    finished: count(TYPING + bot.pace),
    delivered: count(TYPING + bot.pace + FLIGHT),
  } as const
}

export function BotTeam() {
  const main = useQuery(botsQuery).data?.find((bot) => bot.main)
  const reduced = useReducedEffectsMotion()
  const [tick, setTick] = useState(0)
  useEffect(() => {
    const timer = window.setInterval(() => setTick((value) => value + 1), TICK)
    return () => window.clearInterval(timer)
  }, [])
  const moments = TEAM.map((bot, index) => momentAt(bot, index, tick))
  const received = moments.reduce((sum, { delivered }, index) => sum + (TEAM[index].handoff ? delivered : 0), 0)
  return (
    <div className="relative left-1/2 h-[340px] w-[800px] -translate-x-1/2">
      <motion.svg
        aria-hidden
        viewBox="0 0 800 340"
        className="absolute inset-0 text-ink/25"
        initial={reduced ? false : { opacity: 0 }}
        animate={{ opacity: 1, transition: { delay: 0.6, duration: 0.6 } }}
      >
        {TEAM.map((bot) =>
          bot.handoff ? (
            <motion.path
              key={bot.role}
              d={bot.handoff.path}
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              strokeLinecap="round"
              strokeDasharray="0 8"
              animate={reduced ? undefined : { strokeDashoffset: [0, -16] }}
              transition={{ duration: 1.2, repeat: Infinity, ease: 'linear' }}
            />
          ) : null,
        )}
      </motion.svg>
      {TEAM.map((bot, index) => (
        <TeamMember
          key={bot.role}
          bot={bot}
          index={index}
          name={bot.name ?? main?.name ?? 'Prime'}
          avatar={bot.avatar ?? main?.avatar ?? { shape: 'blob', color: 'teal' }}
          moment={moments[index]}
          received={bot.handoff ? 0 : received}
          reduced={reduced}
        />
      ))}
    </div>
  )
}

function TeamMember({
  bot,
  index,
  name,
  avatar,
  moment: { stage, task, turn, finished },
  received,
  reduced,
}: {
  bot: TeamBot
  index: number
  name: string
  avatar: BotAvatarShape
  moment: ReturnType<typeof momentAt>
  received: number
  reduced: boolean
}) {
  const color = BOT_COLORS[avatar.color]
  const Icon = bot.handoff?.icon
  return (
    <>
      <div
        className="absolute flex -translate-x-1/2 flex-col items-center"
        style={{ left: bot.at[0], top: bot.at[1] - bot.size / 2 }}
      >
        <motion.div
          className="relative"
          initial={reduced ? false : { scale: 0.3, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ type: 'spring', bounce: 0.5, duration: 0.7, delay: 0.15 + index * 0.12 }}
        >
          <motion.div
            animate={reduced ? undefined : { y: [0, -5, 0] }}
            transition={{ duration: 3 + index * 0.7, repeat: Infinity, ease: 'easeInOut' }}
          >
            <BotAvatar avatar={avatar} size={bot.size} live completion={`${finished}:${received}`} />
          </motion.div>
          {!reduced && stage === 'done' && finished ? <Burst key={finished} color={color} radius={bot.size * 0.8} /> : null}
          {!reduced && received ? (
            <motion.span
              key={received}
              aria-hidden
              className="pointer-events-none absolute inset-0 rounded-full border-2"
              style={{ borderColor: color }}
              initial={{ scale: 0.9, opacity: 0.6 }}
              animate={{ scale: 1.6, opacity: 0 }}
              transition={{ duration: 0.8, ease: 'easeOut' }}
            />
          ) : null}
        </motion.div>
        <p className="mt-2 text-[13.5px] font-medium leading-tight text-ink">{name}</p>
        <p className="text-[11.5px] text-ink-3">{bot.role}</p>
        {stage === 'hidden' ? null : (
          <motion.div
            className="relative mt-3 whitespace-nowrap rounded-[14px] bg-surface px-3 pb-2 pt-1.5 text-[12px] leading-[18px]"
            style={{ originY: 0 }}
            initial={reduced ? false : { opacity: 0, scale: 0.5, y: -8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            transition={{ type: 'spring', bounce: 0.4, duration: 0.5 }}
          >
            <span aria-hidden className="absolute -top-1 left-1/2 size-2.5 -translate-x-1/2 rotate-45 rounded-[2px] bg-surface" />
            {stage === 'typing' ? (
              <TypingDots />
            ) : stage === 'doing' ? (
              <>
                <Streamed key={`doing-${turn}`} text={task.doing} done={`${task.doing}…`} reduced={reduced} />
                {reduced ? null : (
                  <motion.span
                    key={turn}
                    aria-hidden
                    className="absolute inset-x-3 bottom-[5px] h-[2px] origin-left rounded-full"
                    style={{ background: color }}
                    initial={{ scaleX: 0 }}
                    animate={{ scaleX: 1 }}
                    transition={{ duration: (bot.pace * TICK) / 1000, ease: 'linear' }}
                  />
                )}
              </>
            ) : (
              <span className="flex items-center gap-1.5 text-ink-2">
                <motion.span
                  className="flex size-4 items-center justify-center rounded-full bg-ok text-bg"
                  initial={reduced ? false : { scale: 0, rotate: -90 }}
                  animate={{ scale: 1, rotate: 0 }}
                  transition={{ type: 'spring', bounce: 0.6, duration: 0.5 }}
                >
                  <Check size={10} strokeWidth={3.5} />
                </motion.span>
                <Streamed key={`done-${turn}`} text={task.done} reduced={reduced} />
              </span>
            )}
          </motion.div>
        )}
      </div>
      {Icon && stage === 'done' && !reduced ? (
        <motion.span
          key={finished}
          className="absolute left-0 top-0 flex size-8 items-center justify-center rounded-[9px] bg-ink text-bg shadow-[0_4px_12px_-4px_rgb(0_0_0/0.4)]"
          style={{ offsetPath: `path('${bot.handoff?.path}')`, offsetRotate: '0deg' }}
          initial={{ offsetDistance: '0%', opacity: 0, scale: 0.6 }}
          animate={{ offsetDistance: '100%', opacity: [0, 1, 1, 0], scale: [0.6, 1, 1, 0.5] }}
          transition={{ duration: 1.6, delay: 0.2, ease: 'easeInOut', opacity: HANDOFF_FADE, scale: HANDOFF_FADE }}
        >
          <Icon size={15} />
        </motion.span>
      ) : null}
    </>
  )
}

const HANDOFF_FADE = { duration: 1.6, delay: 0.2, times: [0, 0.12, 0.85, 1] }

// Words arrive one at a time, like a reply streaming in. Once complete, the
// line can carry its finished form (a working line gains its shimmer).
function Streamed({ text, done, reduced }: { text: string; done?: string; reduced: boolean }) {
  const words = text.split(' ')
  const [shown, setShown] = useState(reduced ? words.length : 0)
  useEffect(() => {
    if (shown >= words.length) {
      return
    }
    const timer = window.setTimeout(() => setShown(shown + 1), 70)
    return () => window.clearTimeout(timer)
  }, [shown, words.length])
  if (shown >= words.length) {
    return <span className={done ? 'live-shimmer' : undefined}>{done ?? text}</span>
  }
  return (
    <span className={done ? 'text-ink-3' : undefined}>
      {words.slice(0, shown).map((word, index) => (
        <motion.span key={index} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.2 }}>
          {index ? ` ${word}` : word}
        </motion.span>
      ))}
    </span>
  )
}

function TypingDots() {
  return (
    <span aria-label="Typing" className="flex h-[18px] items-center gap-1 px-0.5">
      {[0, 1, 2].map((index) => (
        <motion.span
          key={index}
          className="size-1.5 rounded-full bg-ink-3"
          animate={{ y: [0, -3, 0], opacity: [0.4, 1, 0.4] }}
          transition={{ duration: 0.9, repeat: Infinity, delay: index * 0.15 }}
        />
      ))}
    </span>
  )
}

// A finished task throws a ring of sparks in the bot's colour.
function Burst({ color, radius }: { color: string; radius: number }) {
  return (
    <span aria-hidden className="pointer-events-none absolute inset-0">
      {Array.from({ length: 8 }, (_, index) => {
        const angle = (index / 8) * Math.PI * 2
        return (
          <motion.span
            key={index}
            className="absolute left-1/2 top-1/2 -ml-[3px] -mt-[3px] size-1.5 rounded-full"
            style={{ background: color }}
            initial={{ x: Math.cos(angle) * radius * 0.55, y: Math.sin(angle) * radius * 0.55, opacity: 1, scale: 1 }}
            animate={{ x: Math.cos(angle) * radius, y: Math.sin(angle) * radius, opacity: 0, scale: 0.3 }}
            transition={{ duration: 0.7, ease: 'easeOut' }}
          />
        )
      })}
    </span>
  )
}
