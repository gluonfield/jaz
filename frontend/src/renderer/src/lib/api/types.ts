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
  status: 'idle' | 'running' | 'error'
  archived?: boolean
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

export type MessageBlock =
  | { type: 'text'; text?: string }
  | { type: 'reasoning'; text?: string }
  | { type: 'tool'; id: string; name: string; input_json?: string; result?: string }

export interface ChatMessage {
  seq: number
  role: 'system' | 'developer' | 'user' | 'assistant'
  content: string
  reasoning?: string
  blocks: MessageBlock[]
  created_at: string
}

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
