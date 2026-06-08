import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, post, put } from './client'
import type { MCPServer, MCPServerInput, MCPServerStatus } from './types'

function normalizeServer(server: MCPServer): MCPServer {
  return {
    ...server,
    headers: server.headers ?? [],
    env_headers: server.env_headers ?? [],
  }
}

function normalizeInput(input: MCPServerInput): MCPServerInput {
  return {
    ...input,
    name: input.name.trim(),
    url: input.url.trim(),
    bearer_token_env_var: input.bearer_token_env_var?.trim() || undefined,
    headers: (input.headers ?? [])
      .map((header) => ({ name: header.name.trim(), value: header.value }))
      .filter((header) => header.name !== ''),
    env_headers: (input.env_headers ?? [])
      .map((header) => ({ name: header.name.trim(), env_var: header.env_var.trim() }))
      .filter((header) => header.name !== '' && header.env_var !== ''),
  }
}

export const mcpServersQuery = queryOptions({
  queryKey: keys.mcpServers,
  queryFn: async () => {
    const data = await get<{ servers: MCPServer[] | null }>('/v1/mcp/servers')
    return (data.servers ?? []).map(normalizeServer)
  },
})

export function createMCPServer(input: MCPServerInput): Promise<MCPServer> {
  return post<MCPServer>('/v1/mcp/servers', normalizeInput(input)).then(normalizeServer)
}

export function updateMCPServer(id: string, input: MCPServerInput): Promise<MCPServer> {
  return put<MCPServer>(`/v1/mcp/servers/${id}`, normalizeInput(input)).then(normalizeServer)
}

export function deleteMCPServer(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/mcp/servers/${id}`)
}

export function setMCPServerEnabled(id: string, enabled: boolean): Promise<MCPServer> {
  return post<MCPServer>(`/v1/mcp/servers/${id}/${enabled ? 'enable' : 'disable'}`).then(
    normalizeServer,
  )
}

export function testMCPServer(id: string): Promise<MCPServerStatus> {
  return post<MCPServerStatus>(`/v1/mcp/servers/${id}/test`)
}

export function authorizeMCPServer(id: string): Promise<MCPServerStatus> {
  return post<MCPServerStatus>(`/v1/mcp/servers/${id}/authorize`)
}
