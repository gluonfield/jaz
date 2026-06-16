// Mirrors of the Go backend's JSON shapes (backend/internal/storage,
// sessionevents, server). Field names must match exactly.

export interface RuntimeRef {
  type: string
  agent?: string
  session_id?: string
  cwd?: string
  project_path?: string
}

// Disjoint components: input is fresh, uncached input; cache reads/writes
// are counted separately, never folded into input.
export interface Usage {
  input_tokens?: number
  cached_input_tokens?: number // cache reads
  cached_write_tokens?: number
  output_tokens?: number
  reasoning_output_tokens?: number
  total_tokens?: number
  // Live context size after the latest turn (replaced per turn, not summed).
  context_tokens?: number
  // Context window reported by the runtime; absent when it doesn't report one.
  context_window_tokens?: number
}

export interface Session {
  id: string
  slug: string
  title?: string
  parent_id?: string
  status: 'idle' | 'running' | 'error'
  error?: string
  archived?: boolean
  pinned?: boolean
  runtime: 'native' | 'acp'
  runtime_ref?: RuntimeRef
  model_provider?: string
  model?: string
  reasoning_effort?: string
  usage?: Usage
  queued_messages?: QueuedMessage[]
  created_at: string
  updated_at: string
  last_attention_at: string
}

export interface ThreadSearchResult {
  thread_id: string
  thread_slug: string
  thread_title?: string
  thread_status?: 'idle' | 'running' | 'error'
  thread_runtime?: 'native' | 'acp'
  parent_id?: string
  archived?: boolean
  message_seq?: number
  snippet?: string
  hit_count?: number
  updated_at: string
  last_attention_at: string
}

export interface QueuedMessage {
  text: string
  attachment_ids?: string[]
  plan_requested?: boolean
}

// Git/forge state of a session's working directory (GET /v1/sessions/:id/repo).
// git=false means "no cwd or not a git repo".
export interface RepoInfo {
  git: boolean
  branch?: string
  default_branch?: string
  remote_url?: string
  web_url?: string
  host?: string
  owner?: string
  repo?: string
  has_upstream?: boolean
  // Confirmed zero commits on top of the default branch (a PR would be empty).
  no_commits?: boolean
  // Commits exist that the remote doesn't have.
  needs_push?: boolean
  dirty?: boolean
  // Linked worktree (not the main checkout); main_branch is the branch the
  // main checkout is on — the handoff destination.
  is_worktree?: boolean
  main_branch?: string
  // Commits on main_branch the worktree's branch doesn't have yet — what
  // "Update from main" would pull in (omitted/0 when up to date).
  behind?: number
  worktree_missing?: boolean
  worktree_restorable?: boolean
  worktree_branch?: string
}

// One changed file in a session's working tree relative to its diff base —
// the worktree's fork point from main, or HEAD in a shared checkout.
export interface RepoFileChange {
  path: string
  // Rename source when status is "renamed".
  old_path?: string
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'untracked'
  added: number
  deleted: number
  binary?: boolean
}

// Identity of a changed file, for React keys and diff-cache keys alike.
// Status is part of it because one path can legitimately be two rows —
// tracked-deleted and recreated untracked.
export function fileKey(file: RepoFileChange): string {
  return `${file.status}:${file.path}`
}

// Numstat-level view of a session's work (GET /v1/sessions/:id/repo/changes):
// which files changed and by how many lines, no patch text.
export interface RepoChanges {
  base?: string
  files: RepoFileChange[]
  total_added: number
  total_deleted: number
}

// One file's unified diff (GET /v1/sessions/:id/repo/diff?path=…), fetched
// only when the user opens the file.
export interface RepoFilePatch {
  path: string
  patch: string
  binary?: boolean
  truncated?: boolean
}

export interface SessionFileRead {
  path: string
  relative_path?: string
  content?: string
  size: number
  binary?: boolean
  truncated?: boolean
}

export interface HealthResponse {
  ok: boolean
  auth_required?: boolean
  capabilities?: {
    session_file_read?: boolean
  }
}

export interface LoopSchedule {
  kind: string
  expr: string
  timezone: string
}

export interface Loop {
  id: string
  name: string
  prompt: string
  schedule: LoopSchedule
  status: 'active' | 'paused' | 'deleted'
  runtime: 'native' | 'acp'
  acp_agent?: string
  model_provider?: string
  model?: string
  reasoning_effort?: string
  directory?: string
  memory_path?: string
  next_run_at?: string
  last_run_at?: string
  last_run_id?: string
  last_run_thread_id?: string
  last_run_status?: string
  last_error?: string
  created_at: string
  updated_at: string
}

export type LoopRunStatus = 'starting' | 'running' | 'ok' | 'error' | 'cancelled' | 'skipped'

export interface LoopRun {
  id: string
  loop_id: string
  thread_id?: string
  scheduled_for: string
  started_at?: string
  finished_at?: string
  status: LoopRunStatus
  error?: string
  created_at: string
}

export interface Board {
  id: string
  name: string
  grid_cols: number
  row_height: number
  // Zooms every widget document on the board (big-screen comfort).
  font_scale: number
  window_bounds?: string
  is_default: boolean
  created_at: string
  updated_at: string
}

// board_widgets joined with the widget and its loop (backend/internal/widgets).
export interface BoardItem {
  board_id: string
  widget_id: string
  x: number
  y: number
  w: number
  h: number
  placed_by: 'llm' | 'user'
  loop_id: string
  loop_name: string
  loop_status: string
  loop_last_run_status?: string
  loop_last_run_at?: string
  title: string
  current_version: number
  size_hint?: string
  last_error?: string
  widget_updated_at: string
}

export interface WidgetSummary {
  id: string
  loop_id: string
  title: string
  current_version: number
  size_hint?: string
  last_error?: string
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
  | {
      type: 'attachment'
      id: string
      name: string
      uri: string
      mime_type?: string
      size?: number
      server_path?: string
    }
  | { type: 'tool'; id: string; name: string; input_json?: string; result?: string }

export interface Attachment {
  id: string
  name: string
  mime_type?: string
  size?: number
  uri: string
  server_path?: string
}

export interface ChatMessage {
  seq: number
  role: 'system' | 'developer' | 'user' | 'assistant'
  content: string
  reasoning?: string
  blocks: MessageBlock[]
  created_at: string
}

// Stored events carry only the acp session id; labels resolve through this
// once-per-response map (old rows may still embed title/slug as a fallback).
export type ACPMeta = Record<string, { title?: string; slug?: string }>

export interface SessionMessages {
  session: Session
  messages: ChatMessage[]
  activity: ActivityEntry[]
  events?: SessionEvent[]
  acp_meta?: ACPMeta
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

export interface PlanEntry {
  content: string
  status?: string
  priority?: string
}

export type ACPPlanEntry = PlanEntry

export interface PlanEvent {
  explanation?: string
  plan?: PlanEntry[]
  awaiting_approval?: boolean
}

export interface ArtifactEvent {
  title: string
  widget_code: string
  loading_messages?: string[]
  artifact_type?: 'svg' | 'html'
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
  plan?: PlanEvent
  permission?: ACPPermission
  artifact?: ArtifactEvent
  at: string
}

export interface AgentFile {
  name: string
  content: string
  exists: boolean
}

export interface MemoryHorizon {
  name: string
  content: string
  chars: number
  max_chars: number
}

export interface MemoryTask {
  name: string
  last_run_at?: string
  status?: string
  error?: string
  next_due?: string
}

export interface MemoryDoctor {
  root: string
  db_path: string
  page_count: number
  chunk_count: number
  link_count: number
  typed_link_count: number
  unresolved_count: number
}

export interface MemoryStatus {
  enabled: boolean
  scheduler_running: boolean
  root: string
  db_path: string
  doctor: MemoryDoctor
  horizons: MemoryHorizon[]
  tasks: MemoryTask[]
  mcp_url?: string
}

export interface MemoryIndexReport {
  page_count: number
  chunk_count: number
  explicit_links: number
  typed_links: number
  mention_links: number
  unresolved_links: number
}

export interface AgentFilesResponse {
  files: AgentFile[]
  root: string
}

export interface MCPHeader {
  name: string
  value: string
}

export interface MCPEnvHeader {
  name: string
  env_var: string
}

export interface MCPServer {
  id: string
  name: string
  transport: 'streamable_http'
  url: string
  enabled: boolean
  bearer_token_env_var?: string
  headers?: MCPHeader[]
  env_headers?: MCPEnvHeader[]
  status: 'connected' | 'disabled' | 'error' | 'needs_auth' | 'unknown'
  tool_count: number
  error?: string
  created_at: string
  updated_at: string
}

export interface MCPServerInput {
  name: string
  url: string
  enabled: boolean
  bearer_token_env_var?: string
  headers?: MCPHeader[]
  env_headers?: MCPEnvHeader[]
}

export interface MCPServerStatus {
  status: 'connected' | 'disabled' | 'error' | 'needs_auth' | 'unknown'
  tool_count: number
  error?: string
  checked_at?: string
}

export interface NativeAgentDefaults {
  model_provider?: string
  model: string
  reasoning_effort?: string
}

export interface NativeProviderOption {
  id: string
  label: string
  base_url: string
  api_key_env?: string
  default_model?: string
  default_reasoning_effort?: string
  implemented: boolean
  opencode?: boolean
  openai_compatible?: boolean
  requires_api_key?: boolean
  /** whether this provider's API key is already configured on the backend */
  configured?: boolean
}

export interface ACPAgentDefaults {
  enabled: boolean
  command?: string
  model?: string
  reasoning_effort?: string
  auth?: ACPAgentAuth
}

export interface ACPAgentAuth {
  mode?: 'auto' | 'existing_cli' | 'jaz_profile'
  path?: string
}

export interface ACPAgentAPIKey {
  source_env?: string
  target_env?: string
}

export interface ACPAgentAuthStatus {
  authenticated: boolean
  reason?: string
  storage_path?: string
  auth_mode?: 'auto' | 'existing_cli' | 'jaz_profile'
  auth_path?: string
  auth_source?: string
  auth_evidence?: string
  auth_kind?: 'oauth' | 'api_key' | 'none'
  recommended_auth?: ACPAgentAuth
  api_key?: ACPAgentAPIKey
  api_key_configured: boolean
  login_command?: string
  login_command_available: boolean
  login_command_reason?: string
  refresh_owner?: string
}

export interface ACPAuthLogin {
  id: string
  agent: string
  status: 'running' | 'succeeded' | 'failed'
  output?: string
  auth_url?: string
  auth_code?: string
  error?: string
  started_at: string
  finished_at?: string
}

export interface ReasoningEffortOption {
  value: string
  label: string
}

export interface ACPAgentOptions {
  reasoning_efforts: ReasoningEffortOption[]
}

export interface AgentSettings {
  native: NativeAgentDefaults
  providers: NativeProviderOption[]
  acp: Record<string, ACPAgentDefaults>
  acp_auth?: Record<string, ACPAgentAuthStatus>
  acp_keys?: Record<string, string>
  acp_options?: Record<string, ACPAgentOptions>
  agents: string[]
}

export interface OnboardingACPProbe extends ACPAgentAuthStatus {
  agent: string
  command?: string
  installed: boolean
  app_installed?: boolean
  app_name?: string
  available: boolean
  auth_command?: string
  auth_command_available: boolean
  auth_command_reason?: string
}

export interface OnboardingNativeProvider {
  id: string
  api_key_env?: string
  configured: boolean
}

export interface OnboardingStatus {
  completed: boolean
  acp: OnboardingACPProbe[]
  native_providers: OnboardingNativeProvider[]
  settings: AgentSettings
}

export interface OnboardingInput {
  settings?: AgentSettings
  provider_keys?: Record<string, string>
  acp_keys?: Record<string, string>
  completed: boolean
}
