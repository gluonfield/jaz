import { createContext } from 'react'
import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'

// MCPAppThread is the thread an app's view lives in: where the view's messages
// go, where the state it reports rides along with the user's next message, and
// where an inline view opens larger.
export type MCPAppThread = {
  id: string
  send: (text: string) => void
  setModelContext?: (view: string, tool: string, text: string) => void
  expand?: (view: string, app: Pick<MCPEntrypoint, 'server_id' | 'tool'>, call: Pick<MCPAppEvent, 'arguments' | 'result'>) => void
}

export const MCPAppThreadContext = createContext<MCPAppThread | null>(null)
