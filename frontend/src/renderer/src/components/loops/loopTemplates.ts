import {
  Activity,
  Brain,
  CalendarClock,
  GitPullRequest,
  type LucideIcon,
  Newspaper,
  ShieldCheck,
} from 'lucide-react'
import { emptyLoopDraft, type LoopDraft } from './LoopForm'
import { defaultScheduleDraft, type ScheduleDraft } from './schedule'

// A curated starting point for a new loop: a ready-to-edit prompt plus a
// sensible schedule. Picking one fills the create form, which the user then
// tweaks — nothing here is special-cased downstream, a template is just a draft.
export interface LoopTemplate {
  id: string
  title: string
  description: string
  icon: LucideIcon
  name: string
  prompt: string
  // Overrides over the daily-9am default; only the keys a template cares about.
  schedule: Partial<ScheduleDraft>
}

export const LOOP_TEMPLATES: LoopTemplate[] = [
  {
    id: 'commit-review',
    title: 'Daily commit review',
    description: 'Summarise the last day of commits and flag anything risky.',
    icon: GitPullRequest,
    name: 'daily-commit-review',
    prompt:
      "Review the commits pushed to this repository in the last 24 hours. Summarise what changed, then flag anything that needs my attention — missing tests, security issues, accidental secrets, or sloppy error handling. Keep it to the few things that actually matter.",
    schedule: { preset: 'weekdays', time: '09:00' },
  },
  {
    id: 'morning-briefing',
    title: 'Morning briefing',
    description: 'A skimmable overnight digest, published to a board widget.',
    icon: Newspaper,
    name: 'morning-briefing',
    prompt:
      'Put together a short morning briefing: the most important overnight developments in AI research and the frontier labs. Group by theme, link sources, and keep it skimmable. Publish it as a widget so I can read it on my board.',
    schedule: { preset: 'daily', time: '07:00' },
  },
  {
    id: 'dependency-watch',
    title: 'Dependency & security watch',
    description: 'New advisories and notable updates for this project.',
    icon: ShieldCheck,
    name: 'dependency-watch',
    prompt:
      'Check this project’s dependencies for security advisories and notable version updates published since yesterday. List only what needs action, each with the affected package, severity, and a one-line recommendation.',
    schedule: { preset: 'daily', time: '08:00' },
  },
  {
    id: 'uptime-check',
    title: 'Endpoint health check',
    description: 'Confirm a site or API is up and responding quickly.',
    icon: Activity,
    name: 'uptime-check',
    prompt:
      'Check that my site is up and responding quickly (replace this with the URL to watch). If it is down or slow, summarise the symptoms and likely cause; otherwise just confirm it is healthy.',
    schedule: { preset: 'hourly' },
  },
  {
    id: 'memory-housekeeping',
    title: 'Memory housekeeping',
    description: 'Capture durable facts and prune stale notes nightly.',
    icon: Brain,
    name: 'memory-housekeeping',
    prompt:
      'Review my recent notes and conversations and update my long-term memory: capture durable facts, decisions, and open loops, and prune anything that has gone stale. Tell me what you changed.',
    schedule: { preset: 'daily', time: '22:00' },
  },
  {
    id: 'weekly-review',
    title: 'Weekly review',
    description: 'What shipped, what is open, and the next three priorities.',
    icon: CalendarClock,
    name: 'weekly-review',
    prompt:
      'Summarise what I shipped and learned this week, what is still open, and the three things most worth doing next week. Keep it honest and concrete.',
    schedule: { preset: 'weekly', time: '16:00', weekday: 5 },
  },
]

export function draftFromTemplate(template: LoopTemplate, boardIds: string[] = []): LoopDraft {
  return {
    ...emptyLoopDraft(boardIds),
    name: template.name,
    prompt: template.prompt,
    schedule: { ...defaultScheduleDraft(), ...template.schedule },
  }
}
