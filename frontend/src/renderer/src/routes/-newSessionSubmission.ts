import { createSession, type CreateSessionInput } from '@/lib/api/sessions'
import type { SendMessageOptions } from '@/lib/sendMessage'
import { appendSessionPrompt } from '@/lib/sessionPrompt'

export async function submitNewSession(
  input: CreateSessionInput,
  text: string,
  options: SendMessageOptions,
  open: (sessionId: string) => void,
): Promise<void> {
  const session = await createSession(input)
  await appendSessionPrompt(session.id, text, options, 'initial')
  open(session.id)
}
