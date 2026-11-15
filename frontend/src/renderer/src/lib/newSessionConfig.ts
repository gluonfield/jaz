export const NEW_SESSION_AGENT_KEY = 'jaz.newSession.explicitAgent'
export const NEW_SESSION_MODELS_KEY = 'jaz.newSession.models'
export const NEW_SESSION_DIRECTORY_KEY = 'jaz.newSession.directory'
export const NEW_SESSION_DRAFT_KEY = 'jaz.newSession.prompt'

export type NewThreadPrompt = { text: string; mode: 'send' | 'draft' }

declare module '@tanstack/history' {
  interface HistoryState {
    newThreadPrompt?: NewThreadPrompt
  }
}
