import { ArrowLeft, ArrowRight, LoaderCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Switch } from '@/components/ui/Switch'
import type { Entrance, Silhouette } from '@/components/launch/DitherArt'
import { onboardingAgentLabel } from '@/lib/agentLabel'

export type SetupStep = 'appearance' | 'agents' | 'memory' | 'connections' | 'computer' | 'voice' | 'bot'

// A slide's dithered hero: its shape and how it arrives.
export type Mark = { draw: Silhouette; enter: Entrance }

// The onboarding motion vocabulary, shared by the gate and the slides.
export const onboardingEase = [0.22, 1, 0.36, 1] as const

export const slideStagger = {
  hidden: {},
  show: { transition: { staggerChildren: 0.07, delayChildren: 0.04 } },
}

export const slideRise = {
  hidden: { opacity: 0, y: 12, filter: 'blur(5px)' },
  show: { opacity: 1, y: 0, filter: 'blur(0px)', transition: { duration: 0.42, ease: onboardingEase } },
  out: { opacity: 0, y: -10, filter: 'blur(6px)', transition: { duration: 0.2, ease: onboardingEase } },
}

// Shared slide footer: Back on the left (from the second slide on), the step
// count dead center, the primary action on the right — the one fixed anchor
// across every slide. The card's lit frame draws the same progress.
export function OnboardingFooter({
  position,
  count,
  nextLabel,
  nextDisabled = false,
  busy = false,
  error,
  onBack,
  onNext,
}: {
  position: number
  count: number
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
        <Button variant="ghost" size="lg" onClick={onBack} className={`justify-self-start ${position === 0 ? 'invisible' : ''}`}>
          <ArrowLeft size={14} />
          Back
        </Button>
        <p className="text-[12px] tabular-nums text-ink-3">
          {position + 1} of {count}
        </p>
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
