import type { ScriptResult } from './script'

export type ComputerStatus = {
  available: boolean
  platform: string
  driverVersion?: string
  permissions?: { accessibility: boolean; screenRecording: boolean }
  owner?: string
  reason?: string
}

export type ComputerAction = { name?: string; args?: Record<string, unknown> }

export interface ComputerAPI {
  status(): Promise<ComputerStatus>
  begin(id: string, session: string): Promise<void>
  call(id: string, action: ComputerAction): Promise<ScriptResult>
  end(id: string): Promise<void>
  cancel(id: string): Promise<void>
}

export const COMPUTER_CHANNEL = 'jaz:computer:'
export const COMPUTER_IMAGE_LIMIT = 32 * 1024 * 1024
