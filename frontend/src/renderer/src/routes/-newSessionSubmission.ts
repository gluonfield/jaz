import { createSession, type CreateSessionInput } from '@/lib/api/sessions'
import type { LiveExchange } from '@/components/session/liveTranscript'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { appendSessionPrompt } from '@/lib/sessionPrompt'

export interface InitialSessionPrompt extends LiveExchange {
  sessionId: string
}

declare module '@tanstack/history' {
  interface HistoryState {
    initialSessionPrompt?: InitialSessionPrompt
  }
}

export async function submitNewSession(
  input: CreateSessionInput,
  text: string,
  options: SendMessageOptions,
  open: (prompt: InitialSessionPrompt) => void,
): Promise<void> {
  const at = new Date().toISOString()
  const session = await createSession(input)
  const uploaded = await appendSessionPrompt(session.id, text, options, 'initial')
  open({
    sessionId: session.id,
    user: text,
    at,
    baselineMessageSeq: 0,
    planRequested: Boolean(options.planRequested),
    goalRequested: Boolean(options.goalRequested),
    contexts: options.contexts ?? [],
    attachments: [...(options.attachments ?? []), ...uploaded],
    reasoning: '',
    assistant: '',
    tools: [],
  })
}
