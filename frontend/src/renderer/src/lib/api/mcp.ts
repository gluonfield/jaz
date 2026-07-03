import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { del, get, post, put } from './client'
import type { MCPServer, MCPServerInput, MCPServerStatus } from './types'

function normalizeServer(server: MCPServer): MCPServer {
  return {
    ...server,
    headers: server.headers ?? [],
    oauth: server.oauth ?? {},
  }
}

function normalizeInput(input: MCPServerInput): MCPServerInput {
  return {
    ...input,
    name: input.name.trim(),
    url: input.url.trim(),
    bearer_token_env_var: input.bearer_token_env_var?.trim() || undefined,
    oauth: normalizeOAuth(input.oauth),
    headers: (input.headers ?? [])
      .map((header) => normalizedHeader(header))
      .filter((header) => header.name !== ''),
  }
}

function normalizedHeader(header: NonNullable<MCPServerInput['headers']>[number]) {
  const name = header.name.trim()
  const envvar = header.envvar?.trim()
  if (envvar) return { name, envvar }
  return { name, value: header.value ?? '' }
}

function normalizeOAuth(oauth: MCPServerInput['oauth']): MCPServerInput['oauth'] | undefined {
  if (!oauth) return undefined
  const out = {
    client_id: oauth.client_id?.trim() || undefined,
    client_secret_env_var: oauth.client_secret_env_var?.trim() || undefined,
    issuer: oauth.issuer?.trim() || undefined,
  }
  return out.client_id || out.client_secret_env_var || out.issuer ? out : undefined
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
