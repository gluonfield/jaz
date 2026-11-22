export type ServerMode = 'cloud' | 'local'
export type ServerAuthMethod = 'oidc' | 'bearer' | 'none'

export interface ServerCapabilities {
  session_file_read?: boolean
  cloud_instructions?: boolean
  connected_machines?: boolean
  native_codex_exec_remote?: boolean
  spaces?: boolean
}

export interface CloudUser {
  id: string
  org_id: string
  org_name?: string
  email: string
  display_name: string
  role: string
}

export type FolderAccessMode = 'ro' | 'rw' | 'rwd'

export interface MachineFolderGrant {
  id: string
  root_path: string
  mode: FolderAccessMode
  label?: string
}

export interface MachineCapabilities {
  file_staging?: boolean
  local_exec?: boolean
  local_browser?: boolean
  computer_use?: boolean
  local_mcps?: string[]
  max_stage_bytes?: number
}

export interface ConnectedMachine {
  device_id: string
  display_name: string
  platform: string
  online: boolean
  capabilities: MachineCapabilities
  folder_grants: MachineFolderGrant[]
  last_seen_at: string
}
