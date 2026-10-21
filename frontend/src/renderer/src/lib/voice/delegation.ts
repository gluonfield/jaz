import type { ChatMessage, SessionMessages } from '@/lib/api/types'
import { voiceReplies, voiceThreadEvents } from '@/lib/voice/transcript'
import type { VoiceWorkActivity } from '@shared/voice'

export interface VoiceTask {
  id: string
  callId: string
  user?: ChatMessage
  progress?: string
  output?: Map<string, string>
  outputSeq?: number
}

export type TaskUpdate = { state: 'running' | 'waiting' | 'approval' | 'completed' | 'failed' | 'cancelled' | 'superseded'; text: string }

export function voiceAgentRunning(snapshot: SessionMessages): boolean {
  return snapshot.session.status === 'running' || ['running', 'starting'].includes(snapshot.acp_state ?? '')
}

function voiceTaskWindow(user: ChatMessage, snapshot: SessionMessages) {
  const after = snapshot.messages.filter((message) => message.seq > user.seq)
  const nextUser = after.find((message) => message.role === 'user')
  const events = voiceThreadEvents(snapshot).filter((event) => Date.parse(event.at) >= Date.parse(user.created_at)
    && (!nextUser || Date.parse(event.at) < Date.parse(nextUser.created_at)))
  return { after, nextUser, events }
}

export function voiceTaskActivity(task: VoiceTask, snapshot: SessionMessages): VoiceWorkActivity {
  const latest = task.user && voiceTaskWindow(task.user, snapshot).events.findLast((event) =>
    ['acp_thought', 'acp_message', 'acp_tool'].includes(event.type),
  )
  return latest?.type === 'acp_thought' ? 'thinking' : 'working'
}

export function voiceReplyChunks(task: VoiceTask, snapshot: SessionMessages): { text: string; seq?: number }[] {
  const replies = task.user ? voiceReplies(snapshot, voiceTaskWindow(task.user, snapshot).events) : []
  const output = task.output ??= new Map()
  const chunks: { text: string; seq?: number }[] = []
  for (const { event: reply, text } of replies) {
    const key = reply.projection_key || reply.acp?.text_run_id || String(reply.seq)
    const sent = output.get(key) ?? ''
    if (!sent && (reply.seq ?? 0) < (task.outputSeq ?? 0)) {
      continue
    }
    if (text === sent) {
      continue
    }
    const delta = text.startsWith(sent) ? text.slice(sent.length) : `Updated agent reply: ${text}`
    if (delta.trim()) {
      chunks.push({ text: delta, seq: reply.seq })
      output.set(key, text)
      task.outputSeq = Math.max(task.outputSeq ?? 0, reply.seq ?? 0)
    }
  }
  return chunks
}

export function voiceTaskUpdate(task: VoiceTask, snapshot: SessionMessages): TaskUpdate {
  task.user ??= snapshot.messages.find((message) => message.role === 'user' && message.blocks?.some((block) =>
    block.type === 'voice_context' && block.id === task.callId && block.request_id === task.id,
  ))
  const user = task.user
  if (!user) {
    if (snapshot.session.status === 'interrupted') return { state: 'cancelled', text: 'The agent task was cancelled.' }
    if (snapshot.session.status === 'error') return { state: 'failed', text: snapshot.acp_error || snapshot.session.error || 'The agent could not start the request.' }
    if (snapshot.session.status === 'idle' && !snapshot.session.pending_steer_message) return { state: 'superseded', text: 'The request was changed or removed in the chat. Check the thread for its current result.' }
    return { state: 'running', text: 'The agent is starting the request.' }
  }
  const { after, nextUser, events } = voiceTaskWindow(user, snapshot)
  const terminal = events.findLast((event) => event.acp && ['idle', 'failed', 'cancelled'].includes(event.acp.state ?? ''))?.acp
  if (nextUser && !terminal) return { state: 'superseded', text: 'The agent is handling a newer instruction in the thread.' }
  const permission = snapshot.acp_permissions?.find((permission) => permission.status === 'pending')
  if (permission?.questions?.length && !nextUser) return { state: 'waiting', text: `Waiting for the user's answer: ${permission.questions.map((question) => question.question).join('\n')}\nPending question: ${JSON.stringify({ request_id: permission.id, questions: permission.questions })}` }
  if (permission && !nextUser) return { state: 'approval', text: `The agent needs your approval in the chat: ${permission.title || 'review the pending request'}.` }
  if (!voiceAgentRunning(snapshot) || terminal) {
    if (terminal?.state === 'cancelled' || (!nextUser && (snapshot.acp_state === 'cancelled' || snapshot.session.status === 'interrupted'))) return { state: 'cancelled', text: 'The agent task was cancelled.' }
    const error = terminal?.error || (!nextUser && (snapshot.acp_error || snapshot.session.error))
    if (error || terminal?.state === 'failed') return { state: 'failed', text: error || 'The agent task failed.' }
    // A bot's saved assistant text is its private narration, so only other
    // agents fall back to it.
    const written = snapshot.session.source_type === 'bot' ? undefined : after.findLast((message) => message.role === 'assistant' && (!nextUser || message.seq < nextUser.seq))?.content
    const answer = voiceReplies(snapshot, events).at(-1)?.text || written
    if (answer) return { state: 'completed', text: answer }
    return { state: 'completed', text: 'The agent finished without a written answer. Check the chat for its tool results.' }
  }
  return { state: 'running', text: 'The agent is working on the request.' }
}

export function taskFinished(update: TaskUpdate): boolean {
  return ['completed', 'failed', 'cancelled', 'superseded'].includes(update.state)
}
