import { queryOptions, useQuery } from '@tanstack/react-query'
import { del, get, post } from '@/lib/api/client'
import { keys } from '@/lib/query/keys'

export interface CloudGitRepoSpec {
  repo: string
  base_branch?: string
  mount_dir?: string
}

export interface CloudThreadGitRepoSpec extends CloudGitRepoSpec {
  branch?: string
}

export interface CloudThreadRepoStatus {
  repo: string
  mount_dir?: string
  base_branch?: string
  branch?: string
  dirty?: boolean
  ahead?: number
  behind?: number
  pr_number?: number
  pr_url?: string
  pr_state?: string
  checks_status?: string
}

type CloudThreadRepoResponse = Partial<CloudThreadRepoStatus> & {
  attached: boolean
  repos?: CloudThreadRepoStatus[]
}

export interface CloudThreadRepoChange {
  path: string
  status: string
  source: string
  additions?: number
  deletions?: number
  size?: number
}

interface CloudThreadRepoChanges {
  repo: string
  base_branch: string
  branch: string
  changes: CloudThreadRepoChange[]
}

interface CloudThreadRepoDiffEntry {
  path: string
  status: string
  source: string
  patch?: string
  content?: string
}

interface CloudThreadRepoDiff {
  diffs: CloudThreadRepoDiffEntry[]
}

interface CloudThreadRepoPushInput {
  repo: string
  branch?: string
  commit_message?: string
}

interface CloudThreadRepoPushResult {
  branch: string
}

interface CloudThreadRepoPRInput {
  repo: string
  title?: string
  body?: string
  head?: string
  base?: string
  draft?: boolean
}

interface CloudThreadRepoPRResult {
  pr_number?: number
  pr_url?: string
  connect_required?: boolean
  compare_url?: string
}

const reposPath = (threadId: string) => `/v1/threads/${encodeURIComponent(threadId)}/repos`
const repoPath = (threadId: string) => `/v1/threads/${encodeURIComponent(threadId)}/repo`

export async function getCloudThreadRepos(threadId: string, signal?: AbortSignal): Promise<CloudThreadRepoStatus[]> {
  const { attached, repos, repo, ...status } = await get<CloudThreadRepoResponse>(repoPath(threadId), { signal })
  if (!attached) return []
  return repos ?? (repo ? [{ ...status, repo }] : [])
}

export async function getCloudThreadRepoChanges(threadId: string, repo: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ repo })
  const response = await get<CloudThreadRepoChanges>(`${repoPath(threadId)}/changes?${query}`, { signal })
  return { ...response, changes: response.changes ?? [] }
}

export async function getCloudThreadRepoDiff(threadId: string, repo: string, path: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ repo, path })
  const response = await get<CloudThreadRepoDiff>(`${repoPath(threadId)}/diff?${query}`, { signal })
  return { ...response, diffs: response.diffs ?? [] }
}

export function pushCloudThreadRepo(threadId: string, input: CloudThreadRepoPushInput) {
  return post<CloudThreadRepoPushResult>(`${repoPath(threadId)}/push`, input)
}

export function createCloudThreadRepoPR(threadId: string, input: CloudThreadRepoPRInput) {
  return post<CloudThreadRepoPRResult>(`${repoPath(threadId)}/pr`, input)
}

export function saveCloudThreadRepo(threadId: string, input: CloudThreadGitRepoSpec) {
  return post(reposPath(threadId), input)
}

export function detachCloudThreadRepo(threadId: string, mountDir: string) {
  return del(`${reposPath(threadId)}/${encodeURIComponent(mountDir)}`)
}

export const cloudThreadReposQuery = (threadId: string) =>
  queryOptions({
    queryKey: keys.cloudThreadRepo(threadId),
    queryFn: ({ signal }) => getCloudThreadRepos(threadId, signal),
    staleTime: 15_000,
    refetchInterval: 30_000,
  })

export const cloudThreadRepoChangesQuery = (threadId: string, repo: string) =>
  queryOptions({
    queryKey: keys.cloudThreadRepoChanges(threadId, repo),
    queryFn: ({ signal }) => getCloudThreadRepoChanges(threadId, repo, signal),
    staleTime: 30_000,
  })

export const cloudThreadRepoDiffQuery = (threadId: string, repo: string, path: string) =>
  queryOptions({
    queryKey: keys.cloudThreadRepoDiff(threadId, repo, path),
    queryFn: ({ signal }) => getCloudThreadRepoDiff(threadId, repo, path, signal),
    staleTime: 30_000,
  })

export function useCloudThreadRepos(threadId: string, enabled = true) {
  return useQuery({
    ...cloudThreadReposQuery(threadId),
    enabled: enabled && Boolean(threadId),
  })
}

export function useCloudThreadRepoChanges(threadId: string, repo: string, enabled: boolean) {
  return useQuery({
    ...cloudThreadRepoChangesQuery(threadId, repo),
    enabled: enabled && Boolean(threadId && repo),
  })
}

export function useCloudThreadRepoDiff(threadId: string, repo: string, path: string, enabled: boolean) {
  return useQuery({
    ...cloudThreadRepoDiffQuery(threadId, repo, path),
    enabled: enabled && Boolean(threadId && repo),
  })
}
