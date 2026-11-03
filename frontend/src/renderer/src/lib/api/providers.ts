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
  const headers = input.headers && Object.keys(input.headers).length ? { ...input.headers } : undefined
  const models = input.models?.length ? [...input.models] : undefined
  const modelsUrl = input.models_url?.trim() || undefined
  return {
    label: input.label.trim(),
    base_url: input.base_url.trim().replace(/\/+$/, ''),
    capabilities: input.capabilities.length ? [...input.capabilities] : ['chat_completions'],
    default_model: input.default_model?.trim() || undefined,
    headers,
    models,
    models_url: modelsUrl,
    icon: input.icon?.trim() || undefined,
    ...(apiKey ? { api_key: apiKey } : {}),
  }
}

export function fetchProviderModels(
  input: ProviderInput & { id?: string; save?: boolean },
): Promise<{ models: string[]; catalog_url?: string; saved?: boolean }> {
  const normalized = normalizeProviderInput(input)
  return post<{ models: string[]; catalog_url?: string; saved?: boolean }>('/v1/providers/fetch-models', {
    id: input.id,
    base_url: normalized.base_url,
    models_url: normalized.models_url,
    headers: normalized.headers,
    save: input.save ?? false,
  })
}

/** Refreshes a stored provider's model catalog and persists the result. */
export function reloadProviderModels(id: string): Promise<{ models: string[]; catalog_url?: string; saved?: boolean }> {
  return post<{ models: string[]; catalog_url?: string; saved?: boolean }>(
    `/v1/providers/fetch-models`,
    { id, save: true },
  )
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
