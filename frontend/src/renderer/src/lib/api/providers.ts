import { del, get, post, put } from './client'
import type {
  ModelProviderOption,
  ModelProviderStatus,
  ModelProviderStatusResponse,
  ProviderInput,
} from './types'

// Trims text fields and drops a blank api_key so an edit that doesn't touch the
// key leaves it unchanged. The endpoint URL keeps its path (unlike MCP's
// origin-collapsing normalize) — providers legitimately have paths like /openai/v1.
function normalizeProviderInput(input: ProviderInput): ProviderInput {
  const apiKey = input.api_key?.trim()
  return {
    label: input.label.trim(),
    base_url: input.base_url.trim().replace(/\/+$/, ''),
    api_type: input.api_type.trim() || 'openai-compatible',
    default_model: input.default_model?.trim() || undefined,
    icon: input.icon?.trim() || undefined,
    ...(apiKey ? { api_key: apiKey } : {}),
  }
}

export function createProvider(input: ProviderInput): Promise<ModelProviderOption> {
  return post<ModelProviderOption>('/v1/providers', normalizeProviderInput(input))
}

export function updateProvider(id: string, input: ProviderInput): Promise<ModelProviderOption> {
  return put<ModelProviderOption>(`/v1/providers/${encodeURIComponent(id)}`, normalizeProviderInput(input))
}

export function getProviderStatus(id: string): Promise<ModelProviderStatus> {
  return get<ModelProviderStatus>(`/v1/providers/${encodeURIComponent(id)}/status`)
}

export function deleteProvider(id: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/v1/providers/${encodeURIComponent(id)}`)
}

export function getProviderStatuses(): Promise<ModelProviderStatusResponse> {
  return get<ModelProviderStatusResponse>('/v1/providers/status')
}
