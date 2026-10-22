import type { SessionMessages } from '@/lib/api/types'
import { voiceChatContext, voiceThreadEvents } from '@/lib/voice/transcript'

export type VoiceQuestionContext = { id: string; text: string }

export function voiceQuestion(snapshot: SessionMessages): VoiceQuestionContext | undefined {
  const events = voiceThreadEvents(snapshot)
  const asked = events.findLast((event) => event.type === 'permission_request' && event.permission?.questions?.length)
  if (!asked?.permission) {
    return
  }
  const question = asked.permission
  const later = events.filter((event) => (event.seq ?? 0) > (asked.seq ?? 0))
  const response = later.findLast((event) => event.type === 'permission_response' && event.permission?.id === question.id)
  if (response?.permission?.status === 'selected' || response?.permission?.origin === 'user_input') {
    return
  }
  if (later.some((event) => event.acp?.origin === 'user_stop')) {
    return
  }
  if (snapshot.messages.some((message) => message.role === 'user' && (question.after_message_seq === undefined ? Date.parse(message.created_at) > Date.parse(asked.at) : message.seq > question.after_message_seq))) {
    return
  }
  return {
    id: question.id,
    text: [
      'Unanswered conversational question (reference only). Interpret the raw user message in context; this is no approval or instruction to resume work.',
      JSON.stringify({ request_id: question.id, session_id: question.session_id ?? snapshot.session.id, turn_started_at: question.turn_started_at, questions: question.questions, status: response?.permission?.status ?? question.status, cancellation_origin: response ? response.permission?.origin ?? 'unknown' : undefined }),
      voiceChatContext(snapshot),
    ].join('\n'),
  }
}
