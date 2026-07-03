import { ArrowRight } from 'lucide-react'
import { motion } from 'motion/react'
import { DitherWordmark, type Silhouette } from '@/components/launch/DitherArt'
import { Button } from '@/components/ui/Button'
import { type OnboardingStep, type SetupStep, onboardingEase, slideExit } from './OnboardingParts'

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

// Everything that defines a setup slide lives in this one table: the dithered
// hero mark, the copy, and where Back/Continue go. The gate only walks it; a
// slide without `next` is the finishing step.
export const SLIDES: Record<
  SetupStep,
  { motif: Silhouette; title: string; subtitle: string; back: OnboardingStep; next?: OnboardingStep }
> = {
  agents: {
    title: 'Connect your agents',
    subtitle: 'jaz runs on the coding agents you already use.',
    back: 'welcome',
    next: 'memory',
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
    back: 'agents',
    next: 'connections',
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
    back: 'memory',
    next: 'loops',
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
  loops: {
    title: 'Always working',
    subtitle: 'Loops run agents on a schedule. Boards show their results live.',
    back: 'connections',
    motif: (g, w, h) => {
      heroGlow(g, w, h)
      g.lineWidth = h * 0.12
      g.beginPath()
      g.arc(w * 0.5, h * 0.5, h * 0.32, 0, Math.PI * 2)
      g.stroke()
      g.globalAlpha = 0.4
      g.lineWidth = h * 0.07
      g.beginPath()
      g.arc(w * 0.5, h * 0.5, h * 0.46, -Math.PI * 0.45, -Math.PI * 0.05)
      g.stroke()
      g.globalAlpha = 1
      g.beginPath()
      g.arc(w * 0.5 + h * 0.46 * Math.cos(-Math.PI * 0.05), h * 0.5 + h * 0.46 * Math.sin(-Math.PI * 0.05), h * 0.1, 0, Math.PI * 2)
      g.fill()
    },
  },
}

// The two footer bits that depend on live state rather than the table.
export function slideFooter(
  step: SetupStep,
  state: { canContinue: boolean; memoryReady: boolean; anyConnected: boolean },
): { nextLabel: string; nextDisabled: boolean } {
  return {
    nextLabel:
      step === 'loops' ? 'Launch jaz' : step === 'connections' && !state.anyConnected ? 'Skip for now' : 'Continue',
    nextDisabled: step === 'agents' ? !state.canContinue : step === 'memory' ? !state.memoryReady : false,
  }
}

// Two miniature product mocks instead of prose: a loop schedule and the live
// board it feeds. The subtitle above them carries the one-line explanation.
export function LoopsBoardsShowcase() {
  return (
    <div className="grid grid-cols-2 gap-2">
      <div className="rounded-[14px] bg-surface p-2.5">
        <div className="flex h-[104px] flex-col justify-center gap-1.5 rounded-[9px] bg-bg px-2.5">
          {(
            [
              ['Morning brief', '8:00'],
              ['Watch CI', '10m'],
              ['Week review', 'Fri'],
            ] as const
          ).map(([name, meta]) => (
            <div key={name} className="flex items-center gap-2 rounded-full bg-surface px-2.5 py-1.5">
              <span className="size-1.5 shrink-0 rounded-full bg-primary" />
              <span className="min-w-0 flex-1 truncate text-[10.5px] font-medium leading-none text-ink-2">{name}</span>
              <span className="shrink-0 font-mono text-[9.5px] leading-none text-ink-3">{meta}</span>
            </div>
          ))}
        </div>
        <p className="mt-2.5 px-1 text-[13px] font-medium text-ink">Loops</p>
        <p className="px-1 pb-0.5 text-[11.5px] leading-snug text-ink-3">Agents on a schedule</p>
      </div>

      <div className="rounded-[14px] bg-surface p-2.5">
        <div className="grid h-[104px] grid-cols-2 gap-1.5 rounded-[9px] bg-bg p-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="flex flex-col justify-center gap-1.5 rounded-[6px] bg-surface px-2">
              <span className={`h-1 rounded-full ${i === 0 ? 'w-3/4 bg-primary/70' : 'w-2/3 bg-ink/20'}`} />
              <span className="h-1 w-1/2 rounded-full bg-ink/10" />
            </div>
          ))}
        </div>
        <p className="mt-2.5 px-1 text-[13px] font-medium text-ink">Boards</p>
        <p className="px-1 pb-0.5 text-[11.5px] leading-snug text-ink-3">Their results, live</p>
      </div>
    </div>
  )
}
