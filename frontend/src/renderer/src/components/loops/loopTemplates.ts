import type { LoopDraft } from './loopDraft'
import { type ScheduleDraft, defaultScheduleDraft } from './schedule'

// A ready-made example loop: a starting prompt plus a sensible schedule. Picking
// one fills the prompt step, which the user then edits — an example only sets
// what it knows (name/prompt/schedule), leaving the agent and boards untouched.
export interface LoopTemplate {
  id: string
  title: string
  description: string
  name: string
  prompt: string
  // Overrides over the daily-9am default; only the keys an example cares about.
  schedule: Partial<ScheduleDraft>
}

export const LOOP_TEMPLATES: LoopTemplate[] = [
  {
    id: 'commit-review',
    title: 'Daily Git commit review',
    description: 'Summarise the last day of commits and flag anything risky.',
    name: 'daily-git-commit-review',
    prompt:
      "Review the commits pushed to this repository in the last 24 hours. Summarise what changed, then flag anything that needs my attention — missing tests, security issues, accidental secrets, or sloppy error handling. Keep it to the few things that actually matter.",
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'linear-priorities',
    title: 'Top Linear issues to tackle',
    description: 'Surface the most important issues assigned to me.',
    name: 'top-linear-issues',
    prompt:
      'Go through my assigned Linear issues and surface the few that matter most right now — weigh priority, due dates, and what’s blocking other work. For each, give a one-line status and the next action.',
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'morning-briefing',
    title: 'Morning AI briefing',
    description: 'A skimmable overnight digest of AI developments.',
    name: 'morning-ai-briefing',
    prompt:
      'Put together a short morning briefing: the most important overnight developments in AI research and the frontier labs. Group by theme, link sources, and keep it skimmable.',
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'world-clocks',
    title: 'World clocks',
    description: 'SF, London, NY, and Dubai as old-school circular clocks.',
    name: 'world-clocks',
    prompt:
      'Show world clocks for SF, London, NY, and Dubai in the style of old-school circular clocks.',
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'dependency-watch',
    title: 'Dependency security advisory watch',
    description: 'New advisories and notable updates for this project.',
    name: 'dependency-security-watch',
    prompt:
      'Check this project’s dependencies for security advisories and notable version updates published since yesterday. List only what needs action, each with the affected package, severity, and a one-line recommendation.',
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'memory-housekeeping',
    title: 'Long-term memory housekeeping',
    description: 'Capture durable facts and prune stale notes nightly.',
    name: 'long-term-memory-housekeeping',
    prompt:
      'Review my recent notes and conversations and update my long-term memory: capture durable facts, decisions, and open loops, and prune anything that has gone stale. Tell me what you changed.',
    schedule: { preset: 'daily', time: '10:00' },
  },
  {
    id: 'weekly-review',
    title: 'Weekly progress review',
    description: 'What shipped, what is open, and the next three priorities.',
    name: 'weekly-progress-review',
    prompt:
      'Summarise what I shipped and learned this week, what is still open, and the three things most worth doing next week. Keep it honest and concrete.',
    schedule: { preset: 'daily', time: '10:00' },
  },
]

// The fields an example contributes to a draft: what to do and when, never the
// agent or board selection the user has already set.
export function templatePatch(template: LoopTemplate): Pick<LoopDraft, 'name' | 'prompt' | 'schedule'> {
  return {
    name: template.name,
    prompt: template.prompt,
    schedule: { ...defaultScheduleDraft(), ...template.schedule },
  }
}
