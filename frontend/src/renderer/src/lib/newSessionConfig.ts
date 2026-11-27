export const NEW_SESSION_AGENT_KEY = 'jaz.newSession.explicitAgent'
export const NEW_SESSION_CLOUD_PROFILE_KEY = 'jaz.newSession.cloudProfile'
export const NEW_SESSION_WORKSPACE_AGENT_KEY = 'jaz.newSession.workspaceAgent'
export const NEW_SESSION_LOCATION_KEY = 'jaz.newSession.location'
export const NEW_SESSION_MODELS_KEY = 'jaz.newSession.models'
export const NEW_SESSION_DIRECTORY_KEY = 'jaz.newSession.directory'
export const NEW_SESSION_DRAFT_KEY = 'jaz.newSession.prompt'

export type NewThreadPrompt = { text: string; files?: File[]; mode: 'send' | 'draft' }

declare module '@tanstack/history' {
  interface HistoryState {
    newThreadPrompt?: NewThreadPrompt
  }
}
