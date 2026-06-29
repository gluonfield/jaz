import type { MCPServer } from '@/lib/api/types'

export function mcpToolCountLabel(count: number): string {
  return `${count} tool${count === 1 ? '' : 's'}`
}

export function mcpStatusText(server: MCPServer): string {
  if (!server.enabled) return 'Disabled'
  if (server.status === 'connected') return mcpToolCountLabel(server.tool_count)
  if (server.status === 'needs_auth') return 'Sign in required'
  if (server.status === 'error') return server.error || 'Connection error'
  return 'Not checked'
}
