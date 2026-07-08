// Mirrors of the Go backend's JSON shapes (backend/internal/storage,
// sessionevents, server). Field names must match exactly.
import type { MessageContextInput } from '@/lib/messageContext'

export interface RuntimeRef {
  type: string
  agent?: string
  session_id?: string
  cwd?: string
  project_path?: string
}

// Provider-facing token fields: input includes cache reads/writes when the
// runtime reports them that way. Cache fields are detail fields.
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

export interface UsageTotals {
  // Raw API input follows provider-facing Usage semantics; display helpers
  // subtract cache read/write before showing it as "Input".
  input_tokens?: number
  cached_input_tokens?: number
  cached_write_tokens?: number
  output_tokens?: number
  reasoning_output_tokens?: number
  input_output_tokens?: number
}

export interface DailyUsage {
  date: string
  usage: UsageTotals
  models?: ModelUsage[]
  categories?: CategoryUsage[]
  session_count: number
}

export interface ModelUsage {
  agent?: string
  model_provider?: string
  model?: string
  usage: UsageTotals
  session_count: number
}

export interface CategoryUsage {
  // chat, loop_run, memory_dream, memory_search, memory_source, browser_task
  category: string
  usage: UsageTotals
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
  runtime: 'acp'
  runtime_ref?: RuntimeRef
  model_provider?: string
  model?: string
  reasoning_effort?: string
  goal?: GoalState
  actions?: SessionActions
  usage?: Usage
  queued_messages?: QueuedMessage[]
  pending_steer_message?: QueuedMessage
  created_at: string
  updated_at: string
  last_attention_at: string
}

export interface SessionActions {
  compact?: boolean
}

export interface FeedMessageTool {
  name?: string
  detail?: string
}

export interface FeedMessage {
  role: string
  text?: string
  tools?: FeedMessageTool[]
  created_at: string
}

// One unread thread on the Feed: its newest message plus identity for the card.
export interface FeedItem {
  id: string
  slug: string
  title?: string
  parent_id?: string
  last_message: FeedMessage
}

export interface ThreadSearchResult {
  thread_id: string
  thread_slug: string
  thread_title?: string
  thread_status?: 'idle' | 'running' | 'error'
  thread_runtime?: 'acp'
  thread_agent?: string
  parent_id?: string
  archived?: boolean
  message_seq?: number
  snippet?: string
  hit_count?: number
  updated_at: string
  last_attention_at: string
}

export interface QueuedMessage {
  id: string
  text: string
  contexts?: MessageContextInput[]
  quotes?: string[]
  attachment_ids?: string[]
  plan_requested?: boolean
  goal_requested?: boolean
}

export type QueuedMessageInput = Omit<QueuedMessage, 'id'> & { id?: string }

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
  // Source used for worktree updates, usually origin/<main_branch>.
  update_branch?: string
  // Commits on update_branch the worktree's branch doesn't have yet — what
  // "Update from main" would pull in (omitted/0 when up to date).
  behind?: number
  // True when the backend can attempt a worktree update, including cases where
  // the remote-tracking ref must be fetched before behind can be known.
  can_update_from_main?: boolean
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

export type IntegrationAuthKind = 'oauth' | 'session' | 'bridge' | 'remote_mcp' | 'browser_local'
export type IntegrationCapability = 'sync' | 'act' | 'materialize' | 'mcp' | 'browser'
export type IntegrationActionRisk = 'read' | 'draft' | 'write' | 'bulk_write' | 'delete'

export interface IntegrationProvider {
  id: string
  name: string
}

export interface IntegrationAuthOption {
  kind: IntegrationAuthKind
  description?: string
  scopes?: string[]
  requires?: string[]
}

export interface IntegrationRemoteMCP {
  url: string
  status: string
  requires?: string[]
  oauth_secrets: boolean
  token_auth?: boolean
}

export interface IntegrationTool {
  name: string
  description: string
  capability: IntegrationCapability
  risk: IntegrationActionRisk
  required_scopes?: string[]
}

export interface IntegrationSkill {
  id: string
  name: string
  description?: string
  status: string
}

export interface IntegrationImplementation {
  status: string
  owner: string
}

export interface IntegrationConnectionAccount {
  id: string
  provider: string
  account_id: string
  account_name?: string
  alias?: string
  scopes?: string[]
  last_synced_at?: string
}

export interface IntegrationConnection {
  status: 'connected' | 'not_connected'
  accounts?: IntegrationConnectionAccount[]
}

export type IntegrationPluginIconKind = 'asset' | 'url' | 'initials'

export interface IntegrationPluginIcon {
  kind: IntegrationPluginIconKind
  value: string
  background?: string
}

export interface IntegrationPlugin {
  id: string
  name: string
  description?: string
  examples?: string[]
  provider: IntegrationProvider
  category?: string
  icon: IntegrationPluginIcon
  auth: IntegrationAuthOption[]
  capabilities: IntegrationCapability[]
  multi_account: boolean
  source_lanes?: string[]
  tools?: IntegrationTool[]
  skills?: IntegrationSkill[]
  remote_mcp?: IntegrationRemoteMCP
  connection_notes?: string[]
  implementation: IntegrationImplementation
  connection?: IntegrationConnection
}

export type ConnectionQRStatusName = 'pending' | 'scanned' | 'password_required' | 'connected' | 'expired' | 'failed'

export interface ConnectionQRStart {
  session_id: string
  provider: string
  code: string
  status: ConnectionQRStatusName
  expires_at: string
  instructions: string[]
}

export interface ConnectionStart {
  type: 'oauth' | 'qr' | 'mcp'
  auth_url?: string
  qr?: ConnectionQRStart
  mcp?: {
    server_id: string
    name: string
    url: string
  }
}

export interface ConnectionQRStatus {
  session_id: string
  provider: string
  code?: string
  status: ConnectionQRStatusName
  expires_at: string
  account_id?: string
  error?: string
}

export type DeviceStatus = 'pending' | 'approved' | 'revoked'
export type DeviceKind = 'desktop' | 'mobile' | 'browser' | 'cli'
export type PairingStatus = 'pending' | 'approved' | 'rejected' | 'expired'

export interface Device {
  id: string
  name: string
  kind: DeviceKind
  status: DeviceStatus
  platform?: string
  device_family?: string
  model_identifier?: string
  created_at: string
  approved_at?: string
  revoked_at?: string
  last_seen_at?: string
  last_seen_ip?: string
  user_agent?: string
  app_version?: string
}

export interface DevicePairing {
  id: string
  device_id: string
  status: PairingStatus
  created_at: string
  expires_at: string
  approved_at?: string
  rejected_at?: string
  device: Device
}

export interface DeviceList {
  devices: Device[]
  pairings: DevicePairing[]
  current_device_id?: string
}

export interface DeviceConnectionLink {
  url: string
}

export interface DeviceRegisterResult {
  device: Device
  token?: string
  pairing?: DevicePairing
  pairing_secret?: string
}

export interface PairingPoll {
  pairing: DevicePairing
  token?: string
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
  runtime: 'acp'
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
  | { type: 'quote'; text?: string; comment?: string }
  | { type: 'browser_annotation'; input_json?: string }
  | {
      type: 'attachment'
      id: string
      name: string
      uri?: string
      mime_type?: string
      size?: number
    }
  | { type: 'tool'; id: string; name: string; input_json?: string; result?: string }

export interface Attachment {
  id: string
  name: string
  mime_type?: string
  size?: number
  uri?: string
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
export type ACPMeta = Record<
  string,
  {
    title?: string
    slug?: string
    model_provider?: string
    model?: string
    reasoning_effort?: string
  }
>

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
  acp_goal_requested?: boolean
  acp_active_operation?: string
  acp_last_event_at?: string
  acp_last_tool_at?: string
  acp_children?: ACPJobSnapshot[]
  acp_child_permissions?: ACPPermission[]
}

export interface ACPToolContent {
  type: 'text' | 'link' | 'diff'
  text?: string
  uri?: string
  title?: string
  path?: string
  old_text?: string
  new_text?: string
}

export interface ACPToolCall {
  id: string
  title?: string
  status?: string
  kind?: string
  tool_name?: string
  content?: ACPToolContent[]
  locations?: ACPToolLocation[]
  raw_input?: unknown
  raw_output?: unknown
  runtime?: ACPToolRuntime
  started_at?: string
  updated_at?: string
}

export interface ACPToolLocation {
  path: string
  line?: number
}

export interface ACPToolRuntime {
  terminal_id?: string
  terminal_cwd?: string
  parent_tool_use_id?: string
  elapsed_time_seconds?: number
  terminal_output_at?: string
  terminal_exit_code?: number
  terminal_exit_signal?: string
}

export interface ACPMode {
  id: string
  name?: string
  description?: string
}

export interface ACPModeState {
  current_mode_id?: string
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

export type GoalStatus =
  | 'requested'
  | 'active'
  | 'paused'
  | 'blocked'
  | 'usageLimited'
  | 'budgetLimited'
  | 'complete'

export interface GoalState {
  id?: string
  thread_id?: string
  objective?: string
  status: GoalStatus
  token_budget?: number
  tokens_used?: number
  remaining_tokens?: number
  time_used_seconds?: number
}

export type GoalEvent = GoalState

export interface ArtifactEvent {
  title: string
  widget_code: string
  loading_messages?: string[]
  artifact_type?: 'svg' | 'html'
}

export interface LoopBoardRef {
  id: string
  name: string
}

export interface LoopCreatedEvent {
  loop_id: string
  loop_name: string
  schedule?: string
  timezone?: string
  next_run_at?: string
  agent?: string
  status?: string
  boards?: LoopBoardRef[]
}

export interface SideChatEvent {
  id: string
  command?: string
  parent_session_id?: string
  thread_id?: string
  role: 'user' | 'assistant' | 'thought' | 'tool' | 'error' | string
  content: string
  status?: string
  contexts?: MessageContextInput[]
  attachments?: Attachment[]
}

export interface ProviderSubagentEvent {
  provider?: string
  id: string
  thread_id?: string
  parent_id?: string
  name?: string
  role?: string
  status?: string
  summary?: string
  prompt?: string
  model?: string
  reasoning_effort?: string
  started_at_ms?: number
  completed_at_ms?: number
}

export interface ACPEvent {
  id: string
  slug: string
  title?: string
  parent_id?: string
  agent: string
  session_id: string
  model_provider?: string
  model?: string
  reasoning_effort?: string
  state: string
  stop_reason?: string
  assistant?: string
  thought?: string
  text_run_id?: string
  error?: string
  modes?: ACPModeState
  plan?: ACPPlanEntry[]
  tool_calls?: ACPToolCall[]
  permissions?: ACPPermission[]
  goal_requested?: boolean
  last_event_at?: string
  last_tool_at?: string
}

export interface ACPJobSnapshot {
  id: string
  slug: string
  title?: string
  parent_id?: string
  acp_agent: string
  acp_session: string
  model_provider?: string
  model?: string
  reasoning_effort?: string
  state: string
  stop_reason?: string
  assistant?: string
  thought?: string
  error?: string
  modes?: ACPModeState
  plan?: ACPPlanEntry[]
  tool_calls?: ACPToolCall[]
  permissions?: ACPPermission[]
  goal_requested?: boolean
  active_operation?: string
  parent_visible?: boolean
  last_event_at?: string
  last_tool_at?: string
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
  content?: string
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
  goal?: GoalEvent
  permission?: ACPPermission
  artifact?: ArtifactEvent
  loop_created?: LoopCreatedEvent
  side_chat?: SideChatEvent
  provider_subagent?: ProviderSubagentEvent
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

export interface MemoryQueueStatus {
  pending: number
  processing: number
  error?: string
}

export interface MemorySourceQueues {
  projection: MemoryQueueStatus
  memory: MemoryQueueStatus
}

export interface MemoryStatus {
  enabled: boolean
  agent?: string
  model?: string
  reasoning_effort?: string
  default_model?: string
  default_reasoning_effort?: string
  scheduler_running: boolean
  root: string
  db_path: string
  doctor: MemoryDoctor
  horizons: MemoryHorizon[]
  tasks: MemoryTask[]
  mcp_url?: string
  source_queues?: MemorySourceQueues
}

export interface MemoryIndexReport {
  page_count: number
  chunk_count: number
  explicit_links: number
  typed_links: number
  mention_links: number
  unresolved_links: number
}

export interface MemoryDreamReport {
  run_slug: string
  review_slug?: string
  input_slugs?: string[]
  promoted: number
  review_items: number
  skipped: number
  long_term_updated?: boolean
  short_term_updated?: boolean
  model_used?: string
  warnings?: string[]
}

export interface MemoryDreamRunResponse {
  index: MemoryIndexReport
  dream: MemoryDreamReport
}

export interface AgentFilesResponse {
  files: AgentFile[]
  root: string
}

export interface MCPHeader {
  name: string
  value?: string
  envvar?: string
}

export interface MCPOAuthConfig {
  client_id?: string
  client_secret_env_var?: string
  issuer?: string
}

export interface MCPServer {
  id: string
  name: string
  transport: 'streamable_http'
  url: string
  enabled: boolean
  bearer_token_env_var?: string
  headers?: MCPHeader[]
  oauth?: MCPOAuthConfig
  status: 'connected' | 'disabled' | 'error' | 'needs_auth' | 'unknown'
  tool_count: number
  tools?: MCPTool[]
  error?: string
  created_at: string
  updated_at: string
}

export interface MCPTool {
  name: string
  remote_name?: string
  description?: string
}

export interface MCPServerInput {
  name: string
  url: string
  enabled: boolean
  bearer_token_env_var?: string
  headers?: MCPHeader[]
  oauth?: MCPOAuthConfig
}

export interface MCPServerStatus {
  status: 'connected' | 'disabled' | 'error' | 'needs_auth' | 'unknown'
  tool_count: number
  tools?: MCPTool[]
  error?: string
  auth_url?: string
  checked_at?: string
}

export type ModelProviderConnectionStatusValue = 'connected' | 'not_connected'

export interface ModelProviderConnectionStatus {
  id: string
  connection_status: ModelProviderConnectionStatusValue
}

export interface ModelProviderStatusResponse {
  providers: ModelProviderConnectionStatus[]
}

export interface ModelProviderOption {
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
  connection_status?: ModelProviderConnectionStatusValue
  /** user-created (DB-backed) provider — editable and deletable in the UI */
  custom?: boolean
  /** API flavor, e.g. "openai-compatible" */
  api_type?: string
  /** icon slug (brand mark) for the provider; falls back to a generic mark */
  icon?: string
}

export interface ModelProviderStatus {
  connection_status: 'connected' | 'not_connected'
}

export interface ModelPricing {
  input: number
  output: number
  cache_read: number
  cache_write: number
}

export interface ModelCatalogEntry {
  value: string
  label: string
  description?: string
  context_length?: number
  pricing?: ModelPricing
  openrouter_id?: string
  reasoning_efforts?: string[] | null
  reasoning_default_effort?: string
  reasoning_mandatory?: boolean
}

/** Editable fields for creating or updating a custom model provider. */
export interface ProviderInput {
  label: string
  base_url: string
  api_type: string
  default_model?: string
  icon?: string
  /** write-only secret; omit on edit to leave an existing key unchanged */
  api_key?: string
}

export interface ACPAgentDefaults {
  enabled: boolean
  command?: string
  model_provider?: string
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
  models?: ModelCatalogEntry[]
  local: boolean
  provider_mode?: 'agent_defaults'
  model_provider_ids?: string[]
  model_providers?: ModelProviderOption[]
  auth_provider_id?: string
  requires_command: boolean
  supports_auth: boolean
}

export interface AgentSettings {
  providers: ModelProviderOption[]
  acp: Record<string, ACPAgentDefaults>
  acp_auth?: Record<string, ACPAgentAuthStatus>
  acp_keys?: Record<string, string>
  acp_options?: Record<string, ACPAgentOptions>
  agents: string[]
}

export interface BrowserExtensionStatus {
  connected: boolean
  extension_id?: string
  protocol?: string
  bridge_url?: string
  user_agent?: string
  actions?: string[]
  last_connected_at?: string
}

export interface BrowserStatus {
  enabled: boolean
  agent?: string
  mode: BrowserMode
  extension: BrowserExtensionStatus
}

export type BrowserMode = 'extension' | 'managed'

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
  managed_adapter?: OnboardingACPAdapterStatus
  managed_tool?: OnboardingManagedToolStatus
}

export interface OnboardingACPAdapterStatus {
  adapter: string
  version?: string
  platform?: string
  state: 'missing' | 'downloading' | 'ready' | 'failed' | 'unsupported'
  message?: string
}

export interface OnboardingManagedToolStatus {
  tool: string
  version?: string
  platform?: string
  state: 'missing' | 'downloading' | 'ready' | 'failed' | 'unsupported'
  path?: string
  message?: string
}

export interface OnboardingMemorySettings {
  enabled: boolean
  agent?: string
}

export interface OnboardingState {
  completed: boolean
}

export interface OnboardingStatus {
  completed: boolean
  acp: OnboardingACPProbe[]
  settings: AgentSettings
  memory: OnboardingMemorySettings
}

export interface OnboardingInput {
  settings?: AgentSettings
  memory?: OnboardingMemorySettings
  provider_keys?: Record<string, string>
  acp_keys?: Record<string, string>
  completed: boolean
}
