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
  error?: string
  archived?: boolean
  runtime: 'native' | 'acp'
  runtime_ref?: RuntimeRef
  queued_messages?: string[]
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
  events?: SessionEvent[]
  acp_state?: string
  acp_assistant?: string
  acp_thought?: string
  acp_modes?: ACPModeState
  acp_plan?: ACPPlanEntry[]
  acp_tool_calls?: ACPToolCall[]
  acp_permissions?: ACPPermission[]
  acp_error?: string
  acp_children?: ACPJobSnapshot[]
}

export interface ACPToolCall {
  id: string
  title?: string
  status?: string
}

export interface ACPMode {
  id: string
  name?: string
  description?: string
}

export interface ACPModeState {
  current_mode_id?: string
  execution_mode_id?: string
  plan_mode_id?: string
  available_modes?: ACPMode[]
}

export interface ACPPlanEntry {
  content: string
  status?: string
  priority?: string
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
  thought?: string
  error?: string
  modes?: ACPModeState
  plan?: ACPPlanEntry[]
  tool_calls?: ACPToolCall[]
  permissions?: ACPPermission[]
}

export interface ACPJobSnapshot {
  id: string
  slug: string
  title?: string
  parent_id?: string
  acp_agent: string
  acp_session: string
  state: string
  stop_reason?: string
  assistant?: string
  thought?: string
  error?: string
  modes?: ACPModeState
  plan?: ACPPlanEntry[]
  tool_calls?: ACPToolCall[]
  permissions?: ACPPermission[]
  parent_visible?: boolean
  updated_at: string
}

export interface ACPPermissionOption {
  id: string
  name: string
  kind?: string
}

export interface ACPPermissionLocation {
  path: string
  line?: number
}

export interface ACPQuestionOption {
  label: string
  description?: string
}

export interface ACPQuestion {
  id: string
  header?: string
  question: string
  is_other?: boolean
  is_secret?: boolean
  options?: ACPQuestionOption[]
}

export interface ACPPermission {
  id: string
  session_id?: string
  title?: string
  tool_call_id?: string
  options?: ACPPermissionOption[]
  locations?: ACPPermissionLocation[]
  questions?: ACPQuestion[]
  status?: string
  selected_option_id?: string
}

export interface SessionEvent {
  seq?: number
  session_id: string
  type: string
  content?: string
  acp?: ACPEvent
  permission?: ACPPermission
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
