import type { MCPServer, MCPServerInput } from '@/lib/api/types'

export type Draft = MCPServerInput & {
  id?: string
  bearer_token_set?: boolean
  argsJSON: string
  envJSON: string
}

export function emptyDraft(): Draft {
  return {
    name: '',
    transport: 'streamable_http',
    url: '',
    enabled: true,
    argsJSON: '[]',
    envJSON: '{}',
  }
}

export function draftFromServer(server: MCPServer): Draft {
  return {
    id: server.id,
    name: server.name,
    transport: server.transport,
    url: server.url,
    command: server.command,
    enabled: server.enabled,
    bearer_token_env_var: server.bearer_token_env_var ?? '',
    bearer_token_set: server.bearer_token_set,
    headers: server.headers ?? [],
    oauth: server.oauth ?? {},
    argsJSON: JSON.stringify(server.args ?? []),
    envJSON: JSON.stringify(server.env ?? {}, null, 2),
  }
}

export function inputFromDraft(draft: Draft): MCPServerInput {
  const { argsJSON, envJSON, ...input } = draft
  if (input.transport !== 'stdio') return input
  let args: unknown
  let env: unknown
  try {
    args = JSON.parse(argsJSON)
    env = JSON.parse(envJSON)
  } catch {
    throw new Error('Arguments and environment must be valid JSON.')
  }
  if (!Array.isArray(args) || !args.every((arg) => typeof arg === 'string')) {
    throw new Error('Arguments must be a JSON array of strings.')
  }
  if (
    !env ||
    typeof env !== 'object' ||
    Array.isArray(env) ||
    !Object.values(env).every((value) => value === null || typeof value === 'string')
  ) {
    throw new Error('Environment must be a JSON object with strings or null.')
  }
  return { ...input, args, env: env as Record<string, string | null> }
}
