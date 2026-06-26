import { runtimeModelState } from '@/lib/agentRuntimes'
import type { CreateSessionInput } from '@/lib/api/sessions'
import type { AgentSettings } from '@/lib/api/types'

// The New-session page and the global launcher both build a create-session
// request from the same last-used choices; keep that resolution here so the two
// surfaces can never drift.
export const NEW_SESSION_AGENT_KEY = 'jaz.newSession.agent'
export const NEW_SESSION_DIRECTORY_KEY = 'jaz.newSession.directory'
export const NEW_SESSION_DRAFT_KEY = 'jaz.newSession.prompt'

export interface NewSessionChoices {
  agent: string
  directory: string
  worktree: boolean
  providerOverride?: string | null
  modelOverride?: string | null
  effortOverride?: string | null
}

export function createSessionInput(
  settings: AgentSettings | undefined,
  choices: NewSessionChoices,
  title?: string,
): CreateSessionInput {
  const model = runtimeModelState(settings, choices.agent, choices.providerOverride ?? null)
  const resolvedModel = choices.modelOverride ?? model.defaultModel
  const resolvedEffort = choices.effortOverride ?? model.defaultEffort
  return {
    ...(title ? { title } : {}),
    runtime: 'acp',
    agent: choices.agent,
    directory: choices.directory,
    worktree: choices.worktree,
    ...(model.usesProvider && model.provider ? { model_provider: model.provider } : {}),
    ...(resolvedModel ? { model: resolvedModel } : {}),
    ...(resolvedEffort ? { reasoning_effort: resolvedEffort } : {}),
  }
}
