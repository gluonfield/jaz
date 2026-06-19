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
    schedule: { preset: 'weekdays', time: '09:00' },
  },
  {
    id: 'morning-briefing',
    title: 'Morning AI briefing',
    description: 'A skimmable overnight digest, published to a board widget.',
    name: 'morning-ai-briefing',
    prompt:
      'Put together a short morning briefing: the most important overnight developments in AI research and the frontier labs. Group by theme, link sources, and keep it skimmable. Publish it as a widget so I can read it on my board.',
    schedule: { preset: 'daily', time: '07:00' },
  },
  {
    id: 'dependency-watch',
    title: 'Dependency security advisory watch',
    description: 'New advisories and notable updates for this project.',
    name: 'dependency-security-watch',
    prompt:
      'Check this project’s dependencies for security advisories and notable version updates published since yesterday. List only what needs action, each with the affected package, severity, and a one-line recommendation.',
    schedule: { preset: 'daily', time: '08:00' },
  },
  {
    id: 'uptime-check',
    title: 'Site & API uptime check',
    description: 'Confirm a site or API is up and responding quickly.',
    name: 'site-api-uptime-check',
    prompt:
      'Check that my site is up and responding quickly (replace this with the URL to watch). If it is down or slow, summarise the symptoms and likely cause; otherwise just confirm it is healthy.',
    schedule: { preset: 'hourly' },
  },
  {
    id: 'memory-housekeeping',
    title: 'Long-term memory housekeeping',
    description: 'Capture durable facts and prune stale notes nightly.',
    name: 'long-term-memory-housekeeping',
    prompt:
      'Review my recent notes and conversations and update my long-term memory: capture durable facts, decisions, and open loops, and prune anything that has gone stale. Tell me what you changed.',
    schedule: { preset: 'daily', time: '22:00' },
  },
  {
    id: 'weekly-review',
    title: 'Weekly progress review',
    description: 'What shipped, what is open, and the next three priorities.',
    name: 'weekly-progress-review',
    prompt:
      'Summarise what I shipped and learned this week, what is still open, and the three things most worth doing next week. Keep it honest and concrete.',
    schedule: { preset: 'weekly', time: '16:00', weekday: 5 },
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
