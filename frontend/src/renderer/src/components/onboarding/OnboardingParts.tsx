import { ArrowLeft, ArrowRight, LoaderCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Switch } from '@/components/ui/Switch'
import { onboardingAgentLabel } from '@/lib/agentLabel'

export type OnboardingStep = 'welcome' | 'agents' | 'memory' | 'connections' | 'loops'
export type SetupStep = Exclude<OnboardingStep, 'welcome'>

// The onboarding motion vocabulary, shared by the gate and the slides.
export const onboardingEase = [0.22, 1, 0.36, 1] as const

export const slideStagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
}

export const slideRise = {
  hidden: { opacity: 0, y: 12, filter: 'blur(5px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.42, ease: onboardingEase } },
}

export const slideExit = {
  opacity: 0,
  y: -6,
  filter: 'blur(3px)',
  transition: { duration: 0.16, ease: onboardingEase },
}

const PROGRESS_STEPS = ['agents', 'memory', 'connections', 'loops'] as const

// Positional pixels in the brand's dither language: square dots, the active
// step stretching into a short bar, past steps tinted toward the brand.
export function OnboardingProgress({ step }: { step: OnboardingStep }) {
  const position = PROGRESS_STEPS.findIndex((value) => value === step)
  return (
    <div aria-label="Setup progress" className="flex shrink-0 items-center gap-[5px]">
      {PROGRESS_STEPS.map((value, index) => (
        <span
          key={value}
          aria-current={index === position ? 'step' : undefined}
          className={`size-[5px] rounded-[1px] transition-all duration-200 ${
            index === position ? 'w-[17px] bg-primary' : index < position ? 'bg-primary/40' : 'bg-ink/15'
          }`}
        />
      ))}
    </div>
  )
}

// Shared slide footer: Back on the left, progress dots dead center, the
// primary action on the right — the one fixed anchor across every slide.
export function OnboardingFooter({
  step,
  nextLabel,
  nextDisabled = false,
  busy = false,
  error,
  onBack,
  onNext,
}: {
  step: OnboardingStep
  nextLabel: string
  nextDisabled?: boolean
  busy?: boolean
  error?: string
  onBack: () => void
  onNext: () => void
}) {
  return (
    <div className="mt-8 w-full">
      {error ? <p className="mb-2 text-center text-pretty text-[12px] text-danger">{error}</p> : null}
      <div className="grid grid-cols-[1fr_auto_1fr] items-center">
        <Button variant="ghost" size="lg" onClick={onBack} className="justify-self-start">
          <ArrowLeft size={14} />
          Back
        </Button>
        <OnboardingProgress step={step} />
        <Button
          variant="primary"
          size="lg"
          disabled={nextDisabled || busy}
          onClick={onNext}
          className="justify-self-end"
        >
          {busy ? <LoaderCircle size={14} className="animate-spin" /> : null}
          {nextLabel}
          {busy ? null : <ArrowRight size={14} />}
        </Button>
      </div>
    </div>
  )
}

export function MemoryCard({
  enabled,
  agent,
  agents,
  onEnabledChange,
  onAgentChange,
}: {
  enabled: boolean
  agent: string
  agents: string[]
  onEnabledChange: (enabled: boolean) => void
  onAgentChange: (agent: string) => void
}) {
  const options = agents.map((value) => ({
    value,
    label: onboardingAgentLabel(value),
  }))
  return (
    <div className="rounded-[14px] bg-surface p-3.5">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13.5px] font-medium text-ink">Remember me</p>
        <Switch checked={enabled} onChange={onEnabledChange} aria-label="Enable memory" />
      </div>
      <div className={enabled ? 'mt-3' : 'pointer-events-none mt-3 opacity-40'}>
        <Select
          value={agent}
          options={options}
          disabled={!enabled || options.length === 0}
          onChange={onAgentChange}
          aria-label="Select memory agent"
          className="h-10 w-full justify-between rounded-[10px] bg-bg px-3 text-[13px]"
        />
        <p className="mt-2 text-[12px] text-ink-3">This agent writes and organizes what jaz remembers.</p>
      </div>
    </div>
  )
}
