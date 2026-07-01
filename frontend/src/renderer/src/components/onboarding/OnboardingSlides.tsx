import { ArrowRight } from 'lucide-react'
import { motion } from 'motion/react'
import { DitherWordmark } from '@/components/launch/DitherArt'
import { Button } from '@/components/ui/Button'
import { onboardingEase } from './OnboardingParts'

// The first thing a new user ever sees: the wordmark assembles itself out of
// glyphs, then one line of copy and the CTA rise in underneath it.
export function WelcomeStep({ onStart }: { onStart: () => void }) {
  return (
    <motion.div
      exit={{ opacity: 0, y: -6, filter: 'blur(3px)', transition: { duration: 0.16, ease: onboardingEase } }}
      className="flex w-full max-w-[440px] flex-col items-center text-center"
    >
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
