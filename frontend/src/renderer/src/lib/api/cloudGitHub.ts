import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import { ApiError } from '@/lib/api/response'
import { openExternal } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import { del, get, patch, post, secureBackendConnection } from './client'
import { patchCloudThread, type CloudThread } from './cloudThreads'

export const DEFAULT_GITHUB_APP_NAME = 'Jaz'
export const GITHUB_CONNECT_LINK = 'jaz://plugin/github/connect'

export interface CloudGitHubInstallation {
  org_id: string
  installation_id: number
  account_login: string
  account_type: string
  scope: 'org' | 'user' | string
  installed_by_user_id?: string | null
  repository_selection: string
  allowed_repos: string[]
  protected_refs: string[]
  suspended_at?: string | null
  updated_at?: string
}

export interface CloudGitHubStatus {
  configured: boolean
  source?: string
  app_id?: number
  app_slug?: string
  app_name?: string
  client_id?: string
  user_linked: boolean
  github_login?: string
  installations: CloudGitHubInstallation[]
  manifest_supported?: boolean
  install_manage_url?: string
}

export interface CloudGitHubManifestStartInput {
  app_name?: string
  github_org?: string
  public?: boolean
  redirect_to?: string
}

export interface CloudGitHubManifestStartResult {
  state: string
  action_url: string
  callback_url: string
  manifest: Record<string, unknown>
}

export interface CloudGitHubInstallStartResult {
  install_url: string
  scope: 'org' | 'user'
  app_slug: string
}

export interface CloudGitHubOAuthStartResult {
  auth_url: string
}

export interface CloudGitHubRepo {
  installation_id: number
  scope: 'org' | 'user' | string
  repo_id: number
  full_name: string
  default_branch: string
  private: boolean
  archived: boolean
  html_url?: string
}

export interface CloudGitHubReposResult {
  repos: CloudGitHubRepo[]
  user_linked: boolean
  connect_required: boolean
  connect_link?: string
}

export interface CloudGitHubBranch {
  name: string
  protected: boolean
  commit?: {
    sha: string
  }
}

export interface CloudGitHubBranchesResult {
  branches: CloudGitHubBranch[]
  connect_required: boolean
  connect_link?: string
}

export interface CloudConnectorAccount {
  id: string
  org_id?: string
  user_id?: string
  server_id: string
  alias: string
  expires_at?: string | null
}

export interface CloudThreadRepo {
  attached: boolean
  repo?: string
  base_branch?: string
  branch?: string
  base_sha?: string
  head_sha?: string
  dirty?: boolean
  has_bundle?: boolean
  ahead?: number
  behind?: number
  pr_number?: number
  pr_url?: string
  pr_state?: string
  checks_status?: string
  sandbox_warm?: boolean
}

export interface CloudThreadRepoChange {
  path: string
  status: string
  source: 'remote_compare' | 'workspace' | string
  additions?: number
  deletions?: number
  size?: number
}

export interface CloudThreadRepoChanges {
  repo: string
  base_branch: string
  branch: string
  changes: CloudThreadRepoChange[]
}

export interface CloudThreadRepoDiffEntry {
  path: string
  status: string
  source: 'remote_compare' | 'workspace' | string
  patch?: string
  content?: string
  additions?: number
  deletions?: number
}

export interface CloudThreadRepoDiff {
  repo: string
  base_branch: string
  branch: string
  diffs: CloudThreadRepoDiffEntry[]
}

export interface CloudThreadRepoPushInput {
  branch?: string
  commit_message?: string
}

export interface CloudThreadRepoPushResult {
  ok: boolean
  repo: string
  branch: string
  head_sha?: string
}

export interface CloudThreadRepoPRInput {
  title?: string
  body?: string
  head?: string
  base?: string
  draft?: boolean
}

export interface CloudThreadRepoPRResult {
  pr_number?: number
  pr_url?: string
  pr_state?: string
  connect_required?: boolean
  connect_link?: string
  compare_url?: string
}

export interface GitHubConnectRequiredError {
  connectRequired: true
  connectLink: string
  message: string
}

const REPO_SEGMENT_RE = /^[A-Za-z0-9._-]+$/

export function splitRepoFullName(fullName: string): { owner: string; repo: string } | null {
  const trimmed = fullName.trim()
  const parts = trimmed.split('/')
  if (parts.length !== 2) return null
  const [owner, repo] = parts
  if (
    !owner ||
    !repo ||
    owner === '.' ||
    owner === '..' ||
    repo === '.' ||
    repo === '..' ||
    !REPO_SEGMENT_RE.test(owner) ||
    !REPO_SEGMENT_RE.test(repo)
  ) {
    return null
  }
  return { owner, repo }
}

export function parseGitHubConnectError(error: unknown): GitHubConnectRequiredError | null {
  if (!(error instanceof ApiError)) return null
  const body = error.body && typeof error.body === 'object' ? (error.body as Record<string, unknown>) : null
  const connectRequired = error.status === 428 || body?.connect_required === true
  if (!connectRequired) return null
  const link = typeof body?.connect_link === 'string' && body.connect_link.trim()
    ? body.connect_link.trim()
    : GITHUB_CONNECT_LINK
  return {
    connectRequired: true,
    connectLink: link,
    message: error.message || 'Connect your GitHub account to continue.',
  }
}

export function submitGitHubManifestForm(
  actionUrl: string,
  manifest: Record<string, unknown>,
  target = '_blank',
): void {
  const trimmedUrl = actionUrl.trim()
  if (!trimmedUrl || !secureBackendConnection(trimmedUrl)) {
    throw new Error('GitHub App manifest action URL must use HTTPS or loopback')
  }
  if (typeof document === 'undefined' || !document.body) {
    throw new Error('Document is unavailable to submit GitHub App manifest form')
  }
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = trimmedUrl
  if (target) form.target = target
  form.style.display = 'none'

  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = 'manifest'
  input.value = JSON.stringify(manifest)
  form.appendChild(input)

  document.body.appendChild(form)
  try {
    form.submit()
  } finally {
    form.remove()
  }
}

function normalizeInstallation(raw: CloudGitHubInstallation): CloudGitHubInstallation {
  return {
    ...raw,
    allowed_repos: Array.isArray(raw.allowed_repos) ? raw.allowed_repos : [],
    protected_refs: Array.isArray(raw.protected_refs) ? raw.protected_refs : [],
  }
}

export async function getCloudGitHubStatus(signal?: AbortSignal): Promise<CloudGitHubStatus> {
  const raw = await get<CloudGitHubStatus>('/v1/github/status', { signal })
  return {
    ...raw,
    configured: Boolean(raw.configured),
    user_linked: Boolean(raw.user_linked),
    installations: Array.isArray(raw.installations) ? raw.installations.map(normalizeInstallation) : [],
  }
}

export async function startCloudGitHubManifest(
  input: CloudGitHubManifestStartInput = {},
): Promise<CloudGitHubManifestStartResult> {
  const appName = input.app_name?.trim() || DEFAULT_GITHUB_APP_NAME
  const githubOrg = input.github_org?.trim() || ''
  const payload: CloudGitHubManifestStartInput = {
    app_name: appName,
    ...(githubOrg ? { github_org: githubOrg } : {}),
    ...(input.public !== undefined ? { public: input.public } : {}),
    ...(input.redirect_to?.trim() ? { redirect_to: input.redirect_to.trim() } : {}),
  }
  return post<CloudGitHubManifestStartResult>('/v1/github/manifest/start', payload)
}

export async function startCloudGitHubInstall(
  scope: 'org' | 'user' = 'org',
  redirectTo?: string,
): Promise<CloudGitHubInstallStartResult> {
  return post<CloudGitHubInstallStartResult>('/v1/github/install/start', {
    scope,
    ...(redirectTo?.trim() ? { redirect_to: redirectTo.trim() } : {}),
  })
}

export async function startCloudGitHubOAuth(redirectTo?: string): Promise<CloudGitHubOAuthStartResult> {
  return post<CloudGitHubOAuthStartResult>('/v1/github/oauth/start', {
    ...(redirectTo?.trim() ? { redirect_to: redirectTo.trim() } : {}),
  })
}

export async function listCloudConnectors(signal?: AbortSignal): Promise<CloudConnectorAccount[]> {
  const response = await get<{ connectors?: CloudConnectorAccount[] | null } | CloudConnectorAccount[]>(
    '/v1/connectors',
    { signal },
  )
  return Array.isArray(response) ? response : (response.connectors ?? [])
}

export async function disconnectCloudGitHubAccount(): Promise<{ disconnected: number }> {
  const connectors = await listCloudConnectors()
  const githubAccounts = connectors.filter((item) => item.server_id === 'github' && item.id)
  for (const account of githubAccounts) {
    await del<{ ok?: boolean }>(`/v1/connectors/${encodeURIComponent(account.id)}`)
  }
  return { disconnected: githubAccounts.length }
}

export async function listCloudGitHubInstallations(signal?: AbortSignal): Promise<CloudGitHubInstallation[]> {
  const response = await get<{ installations?: CloudGitHubInstallation[] | null } | CloudGitHubInstallation[]>(
    '/v1/github/installations',
    { signal },
  )
  const items = Array.isArray(response) ? response : (response.installations ?? [])
  return items.map(normalizeInstallation)
}

export async function patchCloudGitHubInstallation(
  installationId: number,
  input: { allowed_repos?: string[]; protected_refs?: string[] },
): Promise<CloudGitHubInstallation> {
  const updated = await patch<CloudGitHubInstallation>(
    `/v1/github/installations/${encodeURIComponent(String(installationId))}`,
    input,
  )
  return normalizeInstallation(updated)
}

export async function deleteCloudGitHubInstallation(
  installationId: number,
): Promise<{ deleted: boolean; installation_id: number }> {
  return del<{ deleted: boolean; installation_id: number }>(
    `/v1/github/installations/${encodeURIComponent(String(installationId))}`,
  )
}

export async function listCloudGitHubRepos(options: {
  query?: string
  includePersonal?: boolean
  signal?: AbortSignal
} = {}): Promise<CloudGitHubReposResult> {
  const params = new URLSearchParams()
  const q = options.query?.trim() ?? ''
  if (q) params.set('q', q)
  if (options.includePersonal === false) params.set('include_personal', 'false')
  const suffix = params.size ? `?${params.toString()}` : ''
  const raw = await get<CloudGitHubReposResult>(`/v1/github/repos${suffix}`, { signal: options.signal })
  return {
    repos: Array.isArray(raw.repos) ? raw.repos : [],
    user_linked: Boolean(raw.user_linked),
    connect_required: Boolean(raw.connect_required),
    ...(raw.connect_link ? { connect_link: raw.connect_link } : {}),
  }
}

export async function listCloudGitHubBranches(
  owner: string,
  repo: string,
  signal?: AbortSignal,
): Promise<CloudGitHubBranchesResult> {
  try {
    const raw = await get<{ branches?: CloudGitHubBranch[] | null } | CloudGitHubBranch[]>(
      `/v1/github/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches`,
      { signal },
    )
    const branches = Array.isArray(raw) ? raw : (raw.branches ?? [])
    return { branches, connect_required: false }
  } catch (error) {
    const connect = parseGitHubConnectError(error)
    if (connect) {
      return {
        branches: [],
        connect_required: true,
        connect_link: connect.connectLink,
      }
    }
    throw error
  }
}

export async function getCloudThreadRepo(threadId: string, signal?: AbortSignal): Promise<CloudThreadRepo> {
  return get<CloudThreadRepo>(`/v1/threads/${encodeURIComponent(threadId)}/repo`, { signal })
}

export async function getCloudThreadRepoChanges(
  threadId: string,
  signal?: AbortSignal,
): Promise<CloudThreadRepoChanges> {
  const raw = await get<CloudThreadRepoChanges>(`/v1/threads/${encodeURIComponent(threadId)}/repo/changes`, {
    signal,
  })
  return {
    ...raw,
    changes: Array.isArray(raw.changes) ? raw.changes : [],
  }
}

export async function getCloudThreadRepoDiff(
  threadId: string,
  path?: string,
  signal?: AbortSignal,
): Promise<CloudThreadRepoDiff> {
  const params = new URLSearchParams()
  if (path?.trim()) params.set('path', path.trim())
  const suffix = params.size ? `?${params.toString()}` : ''
  const raw = await get<CloudThreadRepoDiff>(`/v1/threads/${encodeURIComponent(threadId)}/repo/diff${suffix}`, {
    signal,
  })
  return {
    ...raw,
    diffs: Array.isArray(raw.diffs) ? raw.diffs : [],
  }
}

export async function pushCloudThreadRepo(
  threadId: string,
  input: CloudThreadRepoPushInput = {},
): Promise<CloudThreadRepoPushResult> {
  const payload: CloudThreadRepoPushInput = {
    ...(input.branch?.trim() ? { branch: input.branch.trim() } : {}),
    ...(input.commit_message?.trim() ? { commit_message: input.commit_message.trim() } : {}),
  }
  return post<CloudThreadRepoPushResult>(`/v1/threads/${encodeURIComponent(threadId)}/repo/push`, payload)
}

export async function createCloudThreadRepoPR(
  threadId: string,
  input: CloudThreadRepoPRInput = {},
): Promise<CloudThreadRepoPRResult> {
  const payload: CloudThreadRepoPRInput = {
    ...(input.title?.trim() ? { title: input.title.trim() } : {}),
    ...(input.body !== undefined ? { body: input.body } : {}),
    ...(input.head?.trim() ? { head: input.head.trim() } : {}),
    ...(input.base?.trim() ? { base: input.base.trim() } : {}),
    ...(input.draft !== undefined ? { draft: input.draft } : {}),
  }
  return post<CloudThreadRepoPRResult>(`/v1/threads/${encodeURIComponent(threadId)}/repo/pr`, payload)
}

export async function attachCloudThreadRepo(
  threadId: string,
  input: { git_repo: string; git_base_branch?: string; git_branch?: string },
): Promise<CloudThread> {
  return patchCloudThread(threadId, {
    git_repo: input.git_repo.trim(),
    ...(input.git_base_branch !== undefined ? { git_base_branch: input.git_base_branch.trim() } : {}),
    ...(input.git_branch !== undefined ? { git_branch: input.git_branch.trim() } : {}),
  })
}

export const cloudGitHubStatusQuery = queryOptions({
  queryKey: keys.cloudGitHubStatus,
  queryFn: ({ signal }) => getCloudGitHubStatus(signal),
  staleTime: 15_000,
})

export const cloudGitHubInstallationsQuery = queryOptions({
  queryKey: keys.cloudGitHubInstallations,
  queryFn: ({ signal }) => listCloudGitHubInstallations(signal),
  staleTime: 15_000,
})

export const cloudGitHubReposQuery = (query = '', includePersonal = true) =>
  queryOptions({
    queryKey: keys.cloudGitHubRepos(query, includePersonal),
    queryFn: ({ signal }) => listCloudGitHubRepos({ query, includePersonal, signal }),
    staleTime: 30_000,
  })

export const cloudGitHubBranchesQuery = (owner: string, repo: string) =>
  queryOptions({
    queryKey: keys.cloudGitHubBranches(owner, repo),
    queryFn: ({ signal }) => listCloudGitHubBranches(owner, repo, signal),
    staleTime: 30_000,
  })

export const cloudThreadRepoQuery = (threadId: string) =>
  queryOptions({
    queryKey: keys.cloudThreadRepo(threadId),
    queryFn: ({ signal }) => getCloudThreadRepo(threadId, signal),
    staleTime: 15_000,
    refetchInterval: 30_000,
  })

export const cloudThreadRepoChangesQuery = (threadId: string) =>
  queryOptions({
    queryKey: keys.cloudThreadRepoChanges(threadId),
    queryFn: ({ signal }) => getCloudThreadRepoChanges(threadId, signal),
    staleTime: 30_000,
  })

export const cloudThreadRepoDiffQuery = (threadId: string, path = '') =>
  queryOptions({
    queryKey: keys.cloudThreadRepoDiff(threadId, path),
    queryFn: ({ signal }) => getCloudThreadRepoDiff(threadId, path, signal),
    staleTime: 30_000,
  })

export function useCloudGitHubStatus(enabled = true, pollActive = false) {
  return useQuery({
    ...cloudGitHubStatusQuery,
    enabled,
    refetchInterval: pollActive ? 2_000 : false,
  })
}

export function useCloudGitHubInstallations(enabled = true) {
  return useQuery({
    ...cloudGitHubInstallationsQuery,
    enabled,
  })
}

export function useCloudGitHubRepos(options: { query?: string; includePersonal?: boolean; enabled?: boolean } = {}) {
  const query = options.query?.trim() ?? ''
  const includePersonal = options.includePersonal !== false
  return useQuery({
    ...cloudGitHubReposQuery(query, includePersonal),
    enabled: options.enabled !== false,
  })
}

export function useCloudGitHubBranches(repoFullName: string, enabled = true) {
  const parsed = splitRepoFullName(repoFullName)
  return useQuery({
    ...cloudGitHubBranchesQuery(parsed?.owner ?? '', parsed?.repo ?? ''),
    enabled: enabled && Boolean(parsed),
  })
}

export function useCloudThreadRepo(threadId: string, enabled = true) {
  return useQuery({
    ...cloudThreadRepoQuery(threadId),
    enabled: enabled && Boolean(threadId),
  })
}

export function useCloudThreadRepoChanges(threadId: string, enabled = true) {
  return useQuery({
    ...cloudThreadRepoChangesQuery(threadId),
    enabled: enabled && Boolean(threadId),
  })
}

export function useCloudThreadRepoDiff(threadId: string, path = '', enabled = true) {
  return useQuery({
    ...cloudThreadRepoDiffQuery(threadId, path),
    enabled: enabled && Boolean(threadId),
  })
}

export function useCloudGitHubAuthRefresh(durationMs = 90_000) {
  const queryClient = useQueryClient()
  const [pollUntil, setPollUntil] = useState(0)

  const refreshNow = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
    void queryClient.invalidateQueries({ queryKey: keys.plugins })
  }, [queryClient])

  const start = useCallback(() => {
    setPollUntil(Date.now() + durationMs)
  }, [durationMs])

  useEffect(() => {
    if (pollUntil === 0) return
    const onFocus = () => {
      if (document.visibilityState === 'hidden') return
      refreshNow()
    }
    const timeout = window.setTimeout(() => setPollUntil(0), Math.max(0, pollUntil - Date.now()))
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onFocus)
    return () => {
      window.clearTimeout(timeout)
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onFocus)
    }
  }, [pollUntil, refreshNow])

  return {
    polling: pollUntil > Date.now(),
    start,
    refreshNow,
  }
}

export function useCloudGitHubOAuth(options: {
  onStarted?: () => void
  onError?: (error: Error) => void
} = {}) {
  const authRefresh = useCloudGitHubAuthRefresh()
  const mutation = useMutation({
    mutationFn: (redirectTo?: string) => startCloudGitHubOAuth(redirectTo),
    onSuccess: (res) => {
      if (!res.auth_url) {
        options.onError?.(new Error('GitHub authorization URL was not returned'))
        return
      }
      authRefresh.start()
      openExternal(res.auth_url)
      options.onStarted?.()
    },
    onError: (error: Error) => {
      options.onError?.(error)
    },
  })
  return {
    ...mutation,
    polling: authRefresh.polling,
  }
}
