import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  attachCloudThreadRepo,
  createCloudThreadRepoPR,
  githubRepoWebUrl,
  openSafeGitHubUrl,
  parseGitHubConnectError,
  pushCloudThreadRepo,
  useCloudGitHubOAuth,
  useCloudThreadRepo,
  type CloudThreadRepoPRInput,
} from '@/lib/api/cloudGitHub'
import { isCloudApiSession } from '@/lib/api/client'
import type { Session } from '@/lib/api/types'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'

export type CloudRepoBusy = 'push' | 'pr' | 'attach' | null

export function useCloudThreadRepoActions(session: Session) {
  const isCloudSession = session.backend === 'cloud' || isCloudApiSession()
  const isViewer = useIsCloudViewer()
  const queryClient = useQueryClient()
  const toast = useToast()

  const [busy, setBusy] = useState<CloudRepoBusy>(null)
  const [connectRequired, setConnectRequired] = useState(false)
  const [compareUrl, setCompareUrl] = useState('')

  useEffect(() => {
    setConnectRequired(false)
    setCompareUrl('')
  }, [session.id])

  const query = useCloudThreadRepo(session.id, isCloudSession)
  const info = query.data

  const oauth = useCloudGitHubOAuth({
    onStarted: () => toast('Finish GitHub sign-in in your browser'),
    onError: (err) => toast(err.message || 'Could not start GitHub sign-in', 'danger'),
  })

  const repoFull = info?.repo ?? session.git_repo ?? ''
  const baseBranch = info?.base_branch ?? session.git_base_branch ?? ''
  const branch = info?.branch ?? session.git_branch ?? ''
  const prNumber = info?.pr_number ?? session.git_pr_number ?? 0
  const prUrl = info?.pr_url ?? session.git_pr_url ?? ''
  const prState = info?.pr_state ?? ''
  const checksStatus = info?.checks_status ?? ''
  const attached = Boolean(info?.attached && repoFull)
  const webUrl = githubRepoWebUrl(repoFull)
  const onBaseBranch = Boolean(branch) && Boolean(baseBranch) && branch === baseBranch

  const invalidateRepo = () => {
    void queryClient.invalidateQueries({ queryKey: keys.cloudThreadRepo(session.id) })
  }

  const openUrl = (url: string) => {
    try {
      openSafeGitHubUrl(url)
    } catch (error) {
      toast((error as Error).message || 'Invalid GitHub URL', 'danger')
    }
  }

  const attachRepo = async (gitRepo: string, gitBaseBranch?: string): Promise<boolean> => {
    if (isViewer) return false
    setBusy('attach')
    try {
      await attachCloudThreadRepo(session.id, {
        git_repo: gitRepo,
        git_base_branch: gitRepo ? (gitBaseBranch ?? '') : '',
      })
      setConnectRequired(false)
      setCompareUrl('')
      invalidateRepo()
      invalidateSessionLists(queryClient, { session: session.id })
      toast(gitRepo ? `Attached ${gitRepo}` : 'Detached repository')
      return true
    } catch (error) {
      toast((error as Error).message || 'Could not update repository', 'danger')
      return false
    } finally {
      setBusy(null)
    }
  }

  const push = async (commitMessage?: string): Promise<boolean> => {
    if (isViewer || !attached) return false
    setBusy('push')
    try {
      const result = await pushCloudThreadRepo(session.id, {
        ...(commitMessage?.trim() ? { commit_message: commitMessage.trim() } : {}),
      })
      setConnectRequired(false)
      invalidateRepo()
      toast(`Pushed ${result.branch || branch || 'branch'}`)
      return true
    } catch (error) {
      const connect = parseGitHubConnectError(error)
      if (connect) {
        setConnectRequired(true)
        toast('Connect your GitHub account before pushing.', 'danger')
      } else {
        toast((error as Error).message || 'Could not push branch', 'danger')
      }
      invalidateRepo()
      return false
    } finally {
      setBusy(null)
    }
  }

  const createOrOpenPR = async (input: CloudThreadRepoPRInput = {}): Promise<boolean> => {
    if (!attached) return false
    if (prUrl) {
      openUrl(prUrl)
      return true
    }
    if (isViewer) return false
    setBusy('pr')
    try {
      const result = await createCloudThreadRepoPR(session.id, input)
      if (result.connect_required && result.compare_url) {
        setConnectRequired(true)
        setCompareUrl(result.compare_url)
        openUrl(result.compare_url)
        toast('Opened GitHub compare page — connect GitHub for 1-click PR creation')
        return true
      }
      if (result.pr_url) {
        setConnectRequired(false)
        setCompareUrl('')
        invalidateRepo()
        openUrl(result.pr_url)
        toast(result.pr_number ? `Pull request #${result.pr_number} ready` : 'Pull request ready')
        return true
      }
      toast('Pull request did not return a URL', 'danger')
      return false
    } catch (error) {
      const connect = parseGitHubConnectError(error)
      if (connect) {
        setConnectRequired(true)
        toast('Connect your GitHub account to create a pull request.', 'danger')
      } else {
        toast((error as Error).message || 'Could not create pull request', 'danger')
      }
      invalidateRepo()
      return false
    } finally {
      setBusy(null)
    }
  }

  const connectGitHub = () => {
    if (isViewer || oauth.isPending) return
    oauth.mutate(undefined)
  }

  const prHint = prUrl
    ? prNumber > 0 ? `Open pull request #${prNumber}` : 'Open pull request'
    : !branch
      ? 'No branch available yet'
      : onBaseBranch
        ? `Push edits onto a feature branch before opening a PR against ${baseBranch}`
        : undefined
  const prDisabled = !prUrl && (isViewer || !branch || onBaseBranch)

  return {
    isCloudSession,
    isViewer,
    info,
    isLoading: query.isLoading,
    error: query.error,
    refetch: () => void query.refetch(),
    attached,
    repo: repoFull,
    baseBranch,
    branch,
    onBaseBranch,
    dirty: Boolean(info?.dirty),
    ahead: info?.ahead ?? 0,
    behind: info?.behind ?? 0,
    prNumber,
    prUrl,
    prState,
    checksStatus,
    sandboxWarm: Boolean(info?.sandbox_warm),
    webUrl,
    busy,
    connectRequired,
    compareUrl,
    connectingGitHub: oauth.isPending,
    prHint,
    prDisabled,
    attachRepo,
    push,
    createOrOpenPR,
    connectGitHub,
    openUrl,
  }
}
