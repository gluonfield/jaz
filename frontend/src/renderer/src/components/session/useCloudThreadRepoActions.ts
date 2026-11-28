import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import { openSafeGitHubUrl, parseGitHubConnectError, useCloudGitHubOAuth } from '@/lib/api/cloudGitHub'
import {
  createCloudThreadRepoPR,
  detachCloudThreadRepo,
  pushCloudThreadRepo,
  saveCloudThreadRepo,
  type CloudGitRepoSpec,
} from '@/lib/api/cloudThreadRepos'
import { keys } from '@/lib/query/keys'

type RepoAction =
  | { kind: 'attach' | 'base'; repo: CloudGitRepoSpec }
  | { kind: 'detach'; repo: string; mountDir: string }
  | { kind: 'push' | 'pr'; repo: string }

type RepoActionResult = { message: string; url?: string; connectRequired?: boolean }

async function applyRepoAction(threadId: string, action: RepoAction): Promise<RepoActionResult> {
  switch (action.kind) {
    case 'attach':
    case 'base':
      await saveCloudThreadRepo(threadId, action.repo)
      return {
        message: action.kind === 'attach' ? `Attached ${action.repo.repo}` : `Updated base branch for ${action.repo.repo}`,
      }
    case 'detach':
      await detachCloudThreadRepo(threadId, action.mountDir)
      return { message: `Detached ${action.repo}` }
    case 'push': {
      const { branch } = await pushCloudThreadRepo(threadId, { repo: action.repo })
      return { message: `Pushed ${action.repo} · ${branch}` }
    }
    case 'pr': {
      const result = await createCloudThreadRepoPR(threadId, { repo: action.repo })
      if (result.pr_url) {
        return {
          message: result.pr_number ? `Pull request #${result.pr_number} ready` : 'Pull request ready',
          url: result.pr_url,
        }
      }
      if (result.connect_required && result.compare_url) {
        return {
          message: 'Opened GitHub compare. Connect GitHub to create pull requests in Jaz.',
          url: result.compare_url,
          connectRequired: true,
        }
      }
      throw new Error('Pull request did not return a URL')
    }
  }
}

export function useCloudThreadRepoActions(threadId: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [connectRequired, setConnectRequired] = useState(false)
  const oauth = useCloudGitHubOAuth({
    onStarted: () => toast('Finish GitHub sign-in in your browser'),
    onError: (err) => toast(err.message, 'danger'),
  })

  function openUrl(url: string) {
    try {
      openSafeGitHubUrl(url)
    } catch (error) {
      toast(error instanceof Error ? error.message : 'Could not open GitHub', 'danger')
    }
  }

  const action = useMutation({
    mutationFn: (input: RepoAction) => applyRepoAction(threadId, input),
    onSuccess: (result) => {
      setConnectRequired(Boolean(result.connectRequired))
      if (result.url) openUrl(result.url)
      toast(result.message)
    },
    onError: (error) => {
      setConnectRequired(Boolean(parseGitHubConnectError(error)))
      toast(error.message, 'danger')
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: keys.cloudThreadRepo(threadId) }),
  })

  async function run(input: RepoAction) {
    try {
      await action.mutateAsync(input)
      return true
    } catch {
      return false
    }
  }

  return {
    busy: action.isPending,
    connectRequired,
    connecting: oauth.isPending,
    connect: () => oauth.mutate(undefined),
    run,
    openUrl,
  }
}
