import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  attachCloudThreadRepo,
  createCloudThreadRepoPR,
  detachCloudThreadRepo,
  openSafeGitHubUrl,
  parseGitHubConnectError,
  pushCloudThreadRepo,
  useCloudGitHubOAuth,
  type CloudThreadGitRepoSpec,
  type CloudThreadRepoStatus,
} from '@/lib/api/cloudGitHub'
import { invalidateSessionLists } from '@/lib/query/invalidate'
import { keys } from '@/lib/query/keys'

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
      toast((error as Error).message, 'danger')
    }
  }

  const action = useMutation({
    mutationFn: (run: () => Promise<string>) => run(),
    onSuccess: (message) => {
      if (message) toast(message)
    },
    onError: (error) => {
      setConnectRequired(Boolean(parseGitHubConnectError(error)))
      toast(error.message, 'danger')
    },
    onSettled: () => {
      invalidateSessionLists(queryClient, { session: threadId })
      return queryClient.invalidateQueries({ queryKey: keys.cloudThreadRepo(threadId) })
    },
  })

  function saveRepo(input: CloudThreadGitRepoSpec, message: string) {
    action.mutate(async () => {
      await attachCloudThreadRepo(threadId, input)
      setConnectRequired(false)
      return message
    })
  }

  function removeRepo(repo: CloudThreadRepoStatus) {
    action.mutate(async () => {
      await detachCloudThreadRepo(threadId, repo.mount_dir || repo.repo)
      setConnectRequired(false)
      return `Detached ${repo.repo}`
    })
  }

  function push(repo: CloudThreadRepoStatus) {
    action.mutate(async () => {
      const result = await pushCloudThreadRepo(threadId, { repo: repo.repo })
      setConnectRequired(false)
      return `Pushed ${repo.repo} · ${result.branch}`
    })
  }

  function openPR(repo: CloudThreadRepoStatus) {
    if (repo.pr_url) {
      openUrl(repo.pr_url)
      return
    }
    action.mutate(async () => {
      const result = await createCloudThreadRepoPR(threadId, { repo: repo.repo })
      setConnectRequired(Boolean(result.connect_required))
      if (result.pr_url) {
        openUrl(result.pr_url)
        return `Pull request #${result.pr_number} ready`
      }
      if (result.connect_required && result.compare_url) {
        openUrl(result.compare_url)
        return 'Opened GitHub compare. Connect GitHub to create pull requests in Jaz.'
      }
      throw new Error('Pull request did not return a URL')
    })
  }

  return {
    busy: action.isPending,
    connectRequired,
    connecting: oauth.isPending,
    connect: () => oauth.mutate(undefined),
    saveRepo,
    removeRepo,
    push,
    openPR,
    openUrl,
  }
}
