// Mirrors of the Go backend's JSON shapes (backend/internal/storage,
// sessionevents, server). Field names must match exactly.

export interface RuntimeRef {
  type: string
  agent?: string
  session_id?: string
}

export interface Session {
  id: string
  slug: string
  title?: string
  parent_id?: string
  runtime: 'native' | 'acp'
  runtime_ref?: RuntimeRef
  created_at: string
  updated_at: string
}

export interface ActivityEntry {
  id?: string
  kind: string
  text?: string
  status?: string
  at: string
}

export interface ToolCallJSON {
  id: string
  type: 'function'
  function: { name: string; arguments: string }
}

// OpenAI-compatible message union (openai-go/v3 on the wire).
export type ChatMessage =
  | { role: 'system' | 'developer'; content: string }
  | { role: 'user'; content: string }
  | { role: 'assistant'; content?: string; tool_calls?: ToolCallJSON[] }
  | { role: 'tool'; tool_call_id: string; content: string }

export interface SessionMessages {
  session: Session
  messages: ChatMessage[]
  activity: ActivityEntry[]
}

export interface ACPToolCall {
  id: string
  title?: string
  status?: string
}

export interface ACPEvent {
  id: string
  slug: string
  title?: string
  parent_id?: string
  agent: string
  session_id: string
  state: string
  stop_reason?: string
  assistant?: string
  error?: string
  tool_calls?: ACPToolCall[]
}

export interface SessionEvent {
  session_id: string
  type: string
  content?: string
  acp?: ACPEvent
  at: string
}

export interface AgentFile {
  name: string
  content: string
  exists: boolean
}

export interface AgentFilesResponse {
  files: AgentFile[]
  root: string
}
