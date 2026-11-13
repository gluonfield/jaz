import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'

// Jaz marks every view it displays presented. Events without the mark predate
// that: those of an entrypoint's tool were shown, and the lookups stay hidden.
export function isPresentedApp(app: MCPAppEvent, entrypoints: MCPEntrypoint[]): boolean {
  return app.presented === true || entrypoints.some((point) => point.server_id === app.server_id && point.tool === app.tool)
}
