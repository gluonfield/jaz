import type { ChatMessage, SessionMessages } from '@/lib/api/types'
import { voiceThreadEvents } from '@/lib/voice/transcript'
import type { VoiceWorkActivity } from '@shared/voice'

export interface VoiceTask {
  id: string
  callId: string
  user?: ChatMessage
  progress?: string
  output?: Map<string, string>
  outputSeq?: number
}

export type TaskUpdate = { state: 'running' | 'approval' | 'completed' | 'failed' | 'cancelled' | 'superseded'; text: string }

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

export function voiceReplyChunks(task: VoiceTask, snapshot: SessionMessages, completed: boolean): string[] {
  const replies = task.user ? voiceTaskWindow(task.user, snapshot).events.filter((event) => event.type === 'acp_message' && event.content) : []
  const output = task.output ??= new Map()
  const chunks: string[] = []
  for (const [index, reply] of replies.entries()) {
    const key = reply.projection_key || reply.acp?.text_run_id || String(reply.seq)
    const text = reply.content!
    const sent = output.get(key) ?? ''
    if (!sent && (reply.seq ?? 0) < (task.outputSeq ?? 0)) {
      continue
    }
    if (text === sent) {
      continue
    }
    const boundary = completed || index < replies.length - 1
      ? text.length
      : [...text.matchAll(/[.!?。！？](?=\s)|\n/g)].at(-1)?.index
    if (boundary === undefined) {
      continue
    }
    const available = text.slice(0, boundary === text.length ? boundary : boundary + 1)
    if (available.length <= sent.length && text.startsWith(sent)) {
      continue
    }
    const delta = available.startsWith(sent) ? available.slice(sent.length) : `Updated agent reply: ${available}`
    if (delta.trim()) {
      chunks.push(delta)
      output.set(key, available)
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
  if (permission && !nextUser) return { state: 'approval', text: `The agent needs your approval in the chat: ${permission.title || 'review the pending request'}.` }
  if (!voiceAgentRunning(snapshot) || terminal) {
    if (terminal?.state === 'cancelled' || (!nextUser && (snapshot.acp_state === 'cancelled' || snapshot.session.status === 'interrupted'))) return { state: 'cancelled', text: 'The agent task was cancelled.' }
    const error = terminal?.error || (!nextUser && (snapshot.acp_error || snapshot.session.error))
    if (error || terminal?.state === 'failed') return { state: 'failed', text: error || 'The agent task failed.' }
    const answer = events.findLast((event) => event.type === 'acp_message')?.content || after.findLast((message) => message.role === 'assistant' && (!nextUser || message.seq < nextUser.seq))?.content
    if (answer) return { state: 'completed', text: answer }
    return { state: 'completed', text: 'The agent finished without a written answer. Check the chat for its tool results.' }
  }
  const lastTool = snapshot.acp_tool_calls?.at(-1)
  return { state: 'running', text: lastTool?.title ? `The agent is working: ${lastTool.title}` : 'The agent is working on the request.' }
}

export function taskFinished(update: TaskUpdate): boolean {
  return ['completed', 'failed', 'cancelled', 'superseded'].includes(update.state)
}
