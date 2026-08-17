import { createSession, type CreateSessionInput } from '@/lib/api/sessions'
import { userInputMessageBlocks } from '@/lib/messageBlocks'
import type { OptimisticUserMessage } from '@/lib/optimisticUserMessage'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { appendSessionPrompt } from '@/lib/sessionPrompt'

export interface InitialSessionPrompt extends OptimisticUserMessage {
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
  const appended = await appendSessionPrompt(session.id, text, options, 'initial')
  if (!appended) throw new Error('Cannot start a session without a message')
  open({
    sessionId: session.id,
    baselineMessageSeq: 0,
    message: {
      role: 'user',
      content: appended.message.text,
      blocks: userInputMessageBlocks(
        appended.message.text,
        appended.message.contexts,
        appended.attachments,
      ),
      created_at: at,
    },
  })
}
