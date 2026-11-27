import { queryOptions } from '@tanstack/react-query'
import { request } from '@/lib/api/client'
import { cloudWorkspaceHeaders } from '@/lib/api/cloudScope'
import { ApiError } from '@/lib/api/response'
import { keys } from '@/lib/query/keys'

export type VaultKind = 'login' | 'card' | 'identity'

export interface VaultItem {
  id: string
  kind: VaultKind
  label: string
  origins?: string[]
  username?: string
  has_totp?: boolean
  card_brand?: string
  card_last4?: string
  card_exp_month?: number
  card_exp_year?: number
  cardholder_name?: string
  billing_summary?: string
  email?: string
  city_country?: string
  created_at: string
  updated_at: string
}

export interface VaultItemInput {
  kind?: VaultKind
  label?: string
  origins?: string[]
  replace_origins?: boolean
  username?: string
  password?: string
  totp_secret?: string
  cardholder_name?: string
  pan?: string
  card_exp_month?: number
  card_exp_year?: number
  cvv?: string
  billing_address?: string
  billing_summary?: string
  full_name?: string
  email?: string
  phone?: string
  address_lines?: string
  postal_code?: string
  country?: string
  city_country?: string
}

const vaultItemsPath = '/v1/vault/items'

export async function listVaultItems(options?: {
  kind?: VaultKind | ''
  domain?: string
  workspaceID?: string
  signal?: AbortSignal
}): Promise<VaultItem[]> {
  const params = new URLSearchParams()
  if (options?.kind) params.set('kind', options.kind)
  if (options?.domain) params.set('domain', options.domain)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  try {
    const res = await request<{ items?: VaultItem[] | null }>(`${vaultItemsPath}${suffix}`, {
      signal: options?.signal,
      headers: cloudWorkspaceHeaders(options?.workspaceID),
    })
    return res.items ?? []
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return []
    }
    throw error
  }
}

export const vaultItemsQuery = (kind: VaultKind | '' = '', domain = '', workspaceID?: string) =>
  queryOptions({
    queryKey: keys.vaultItems(kind, domain, workspaceID ?? ''),
    queryFn: ({ signal }) => listVaultItems({ kind, domain, workspaceID, signal }),
    staleTime: 15_000,
    retry: false,
  })

export async function createVaultItem(
  input: VaultItemInput & { kind: VaultKind },
  workspaceID?: string,
  signal?: AbortSignal,
): Promise<VaultItem> {
  const res = await request<{ item: VaultItem }>(vaultItemsPath, {
    method: 'POST',
    headers: { ...cloudWorkspaceHeaders(workspaceID), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  })
  return res.item
}

export async function updateVaultItem(
  id: string,
  input: VaultItemInput,
  workspaceID?: string,
  signal?: AbortSignal,
): Promise<VaultItem> {
  const res = await request<{ item: VaultItem }>(`${vaultItemsPath}/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { ...cloudWorkspaceHeaders(workspaceID), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  })
  return res.item
}

export async function deleteVaultItem(id: string, workspaceID?: string, signal?: AbortSignal): Promise<void> {
  await request<{ deleted: boolean }>(`${vaultItemsPath}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: cloudWorkspaceHeaders(workspaceID),
    signal,
  })
}
