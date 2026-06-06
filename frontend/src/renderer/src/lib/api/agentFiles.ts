import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get, put } from './client'
import type { AgentFile, AgentFilesResponse } from './types'

export const agentFilesQuery = queryOptions({
  queryKey: keys.agentFiles,
  queryFn: () => get<AgentFilesResponse>('/v1/agent/files'),
})

export function saveAgentFile(name: string, content: string): Promise<AgentFile> {
  return put<AgentFile>(`/v1/agent/files/${encodeURIComponent(name)}`, { content })
}
