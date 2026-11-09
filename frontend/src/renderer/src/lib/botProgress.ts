import type { ACPActivity, ACPPlanEntry, ACPToolCall } from '@/lib/api/types'

export type BotProgressKind =
  | 'processing'
  | 'starting'
  | 'reasoning'
  | 'responding'
  | 'error'
  | 'reading'
  | 'searching'
  | 'running_command'
  | 'editing'
  | 'using_tool'
  | 'waiting'

export interface BotProgress {
  kind: BotProgressKind
  label: string
  detail?: string
  toolDetail?: string
  planStep?: string
  ageMs?: number
  stale: boolean
  waitReason?: string
}

const STALE_MS = 120_000

const LABELS: Record<BotProgressKind, string> = {
  processing: 'Waiting for activity details',
  starting: 'Waiting for the first agent event',
  reasoning: 'Reasoning',
  responding: 'Responding',
  error: 'Agent error',
  reading: 'Reading files',
  searching: 'Searching the web',
  running_command: 'Running a command',
  editing: 'Editing files',
  using_tool: 'Using a tool',
  waiting: 'Waiting for the next event',
}

const TOOL_LABELS: Record<string, string> = {
  bash: 'terminal',
  execcommand: 'terminal',
  read: 'file read',
  readfile: 'file read',
  ls: 'file listing',
  glob: 'file search',
  grep: 'text search',
  websearch: 'web search',
  webfetch: 'page fetch',
  todowrite: 'plan update',
  updateplan: 'plan update',
}

function eventTime(iso: string | undefined): number {
  const time = Date.parse(iso ?? '')
  return Number.isFinite(time) && time > 0 ? time : 0
}

function latestTool(toolCalls: ACPToolCall[] | undefined): ACPToolCall | undefined {
  if (!toolCalls?.length) return undefined
  const active = toolCalls.filter(isActiveTool)
  return (active.length ? active : toolCalls).reduce<ACPToolCall | undefined>((latest, call) => {
    const a = eventTime(call.updated_at ?? call.started_at)
    const b = eventTime(latest?.updated_at ?? latest?.started_at)
    return a >= b ? call : latest
  }, undefined)
}

function isActiveTool(call: ACPToolCall): boolean {
  return call.status === 'pending' || call.status === 'in_progress' || call.status === 'running'
}

function categoryOf(call: ACPToolCall | undefined): BotProgressKind {
  if (!call) return 'processing'
  const kind = (call.kind ?? '').toLowerCase()
  const name = (call.tool_name ?? call.title ?? '').toLowerCase()
  if (kind === 'execute' || name.includes('bash') || name.includes('terminal')) return 'running_command'
  if (name.includes('websearch') || name.includes('web search') || name.includes('xsearch')) return 'searching'
  if (kind === 'read' || kind === 'search' || kind === 'fetch') return 'reading'
  if (kind === 'edit' || kind === 'write') return 'editing'
  return 'using_tool'
}

function toolDetail(call: ACPToolCall | undefined): string | undefined {
  if (!call) return undefined
  const name = (call.tool_name ?? '').toLowerCase().replace(/[\s_-]/g, '')
  if (TOOL_LABELS[name]) return TOOL_LABELS[name]
  if (call.kind === 'execute') return 'terminal'
  if (call.kind === 'read') return 'file read'
  if (call.kind === 'search') return 'search'
  return 'tool'
}

function activePlanStep(plan: ACPPlanEntry[] | undefined): string | undefined {
  const step = plan?.find((entry) => entry.status?.toLowerCase() === 'in_progress')?.content.trim()
  if (!step) return undefined
  return step.length > 120 ? `${step.slice(0, 117)}…` : step
}

const PREVIEW_EXECUTABLES = new Set([
  'bash', 'sh', 'zsh', 'pwsh', 'git', 'go', 'bun', 'npm', 'npx', 'yarn', 'pnpm',
  'node', 'python', 'python3', 'pip', 'pip3', 'uv', 'flutter', 'dart', 'xcrun',
  'xcodebuild', 'docker', 'ssh', 'scp', 'curl', 'wget', 'ls', 'rg', 'find',
  'cat', 'cp', 'mv', 'tar', 'zip', 'unzip', 'jq', 'sed', 'awk', 'systemctl', 'journalctl',
])

function commandPreview(call: ACPToolCall | undefined): string | undefined {
  if (!call || categoryOf(call) !== 'running_command') return undefined
  if (call.program && PREVIEW_EXECUTABLES.has(call.program)) return call.program
  if (!call.raw_input || typeof call.raw_input !== 'object') return undefined
  const input = call.raw_input as Record<string, unknown>
  const raw = input.command ?? input.cmd ?? input.script
  // Arbitrary shell arguments cannot be reliably scrubbed with regexes. Only
  // display a known bare executable; paths, wrappers and all arguments stay private.
  const executable = Array.isArray(raw) ? raw[0] : typeof raw === 'string' ? raw.trimStart().split(/\s/, 1)[0] : undefined
  return typeof executable === 'string' && PREVIEW_EXECUTABLES.has(executable) ? executable : undefined
}

export type AgentProgressInput = {
  running: boolean
  activity?: ACPActivity
  lastEventAt?: string
  lastToolAt?: string
  toolCalls?: ACPToolCall[]
  plan?: ACPPlanEntry[]
  now: number
}

export function deriveBotProgress(input: AgentProgressInput & { running: true }): BotProgress
export function deriveBotProgress(input: AgentProgressInput): BotProgress | undefined
export function deriveBotProgress(input: AgentProgressInput): BotProgress | undefined {
  if (!input.running && input.activity?.phase !== 'error') return undefined
  const activity = input.activity
  const turnAt = eventTime(activity?.turn_started_at)
  const tools = turnAt ? input.toolCalls?.filter((tool) => eventTime(tool.updated_at ?? tool.started_at) >= turnAt) : input.toolCalls
  const tool = latestTool(tools)
  const toolActive = !!tool && isActiveTool(tool)
  const toolAt = eventTime(tool?.updated_at ?? tool?.started_at)
  const phaseAt = eventTime(activity?.at)
  let kind: BotProgressKind = toolActive ? categoryOf(tool) : tool ? 'waiting' : 'processing'
  if (activity) {
    if (activity.phase === 'error') kind = 'error'
    else if (activity.phase === 'starting') kind = toolActive ? categoryOf(tool) : 'starting'
    else if (activity.phase === 'reasoning' || activity.phase === 'responding') {
      if (!toolActive || phaseAt >= toolAt) kind = activity.phase
    } else if (activity.phase === 'waiting' && activity.reason !== 'tool_finished') kind = 'waiting'
    else if (!toolActive) kind = 'waiting'
  }
  const actionPrefix = toolActive ? undefined : tool ? 'Last action:' : undefined
  const command = commandPreview(tool)
  const toolDescription = [actionPrefix, toolDetail(tool), command ? `› ${command}` : undefined].filter(Boolean).join(' · ') || undefined
  const planStep = activePlanStep(input.plan)
  const detail = [toolDescription, planStep].filter(Boolean).join(' · ') || undefined
  const at = Math.max(phaseAt, eventTime(input.lastEventAt), eventTime(input.lastToolAt), toolAt)
  const ageMs = at ? Math.max(0, input.now - at) : undefined
  const reason = activity?.reason
  const waitReason = reason === 'provider_error' ? 'The agent reported an error'
    : reason === 'provider_retry' ? 'The provider reported a retry'
    : reason === 'interrupted' ? 'Agent connection interrupted'
    : reason === 'user_input' ? 'Waiting for your answer'
    : reason === 'tool_finished' ? 'Tool finished; waiting for the next event'
    : kind === 'starting' ? 'Request accepted; no agent event received yet'
    : undefined
  return { kind, label: LABELS[kind], detail, toolDetail: toolDescription, planStep, ageMs, stale: ageMs !== undefined && ageMs > STALE_MS, waitReason }
}
