import { ArrowRight } from 'lucide-react'
import { motion } from 'motion/react'
import type { SystemPermission } from '@shared/systemPermissions'
import { DitherWordmark, type Silhouette } from '@/components/launch/DitherArt'
import { PaletteIdeas } from '@/components/settings/ColorThemeSettings'
import { ThemeModePicker } from '@/components/settings/ThemeModePicker'
import { Button } from '@/components/ui/Button'
import { type Mark, type SetupStep, onboardingEase, slideExit } from './OnboardingParts'

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
// hero mark and how it arrives, the copy, and the system permissions it asks
// for. The bot slide has no mark: its bots are the picture.
export const SLIDES: Record<
  SetupStep,
  { mark?: Mark; title: string; subtitle: string; permissions?: SystemPermission[] }
> = {
  appearance: {
    title: 'Make it yours',
    subtitle: 'Pick a mode and a color. Fine-tune both any time in Settings.',
    mark: {
      draw: (g, w, h) => {
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
      // A dial: the dots gather in a bright core and sweep out clockwise from noon.
      enter: (x, y, w, h) => ({ x: w / 2, y: h / 2, at: (((Math.atan2(y - h / 2, x - w / 2) + Math.PI / 2) / (2 * Math.PI) + 1) % 1) }),
    },
  },
  agents: {
    title: 'Connect your agents',
    subtitle: 'jaz runs on the coding agents you already use.',
    mark: {
      draw: (g, w, h) => {
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
      // Typed: a pen point at the prompt's start draws the chevron top to bottom, then the cursor.
      enter: (x, y, w, h) => ({ x: w * 0.41, y: h * 0.24, at: x > w * 0.55 ? 1 : (0.85 * y) / h }),
    },
  },
  memory: {
    title: 'Give jaz a memory',
    subtitle: 'Preferences, decisions, projects — agents start warm, not cold.',
    mark: {
      draw: (g, w, h) => {
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
      // Stacked: the layers drop in from above, bottom first.
      enter: (x, y, w, h) => ({ x, y: y - h, at: y > h * 0.62 ? 0 : y > h * 0.34 ? 0.5 : 1 }),
    },
  },
  connections: {
    title: 'Connect your world',
    subtitle: 'Your email and chats become context agents can use.',
    mark: {
      draw: (g, w, h) => {
        heroGlow(g, w, h)
        g.lineWidth = h * 0.12
        for (const x of [0.43, 0.57]) {
          g.beginPath()
          g.arc(w * x, h * 0.5, h * 0.3, 0, Math.PI * 2)
          g.stroke()
        }
      },
      // Linked: the rings slide in from either side and snap together.
      enter: (x, y, w, h) => ({ x: x < w / 2 ? x - w * 0.3 : x + w * 0.3, y, at: Math.abs(y - h / 2) / h }),
    },
  },
  computer: {
    title: 'Let jaz use your computer',
    subtitle: 'Agents can click, type and read apps to finish tasks while you are away.',
    permissions: ['accessibility', 'screenRecording'],
    mark: {
      draw: (g, w, h) => {
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
      // The screen powers on from a line, then the cursor swoops in.
      enter: (x, y, w, h) => (x > w * 0.53 && y > h * 0.4 ? { x: x + w * 0.25, y: y + h * 0.6, at: 1 } : { x, y: h * 0.43, at: 0 }),
    },
  },
  voice: {
    title: 'Talk to jaz',
    subtitle: 'Speak instead of typing, and get notified when a thread finishes.',
    permissions: ['microphone', 'notifications'],
    mark: {
      draw: (g, w, h) => {
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
      // Spoken: the dots flatten onto a line and the bars spring out of it from the middle.
      enter: (x, y, w, h) => ({ x, y: h / 2, at: Math.min(1, Math.abs(x - w / 2) / (w * 0.22)) }),
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
        <PaletteIdeas />
      </div>
    </>
  )
}
