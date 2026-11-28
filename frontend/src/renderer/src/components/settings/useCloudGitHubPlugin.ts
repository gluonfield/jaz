import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  CLOUD_GITHUB_PLUGIN_ID,
  buildCloudGitHubPlugin,
  disconnectCloudGitHubAccount,
  isCloudAuthExpiredError,
  useCloudGitHubAuthRefresh,
  useCloudGitHubOAuth,
  useCloudGitHubStatus,
} from '@/lib/api/cloudGitHub'
import { keys } from '@/lib/query/keys'

export const CLOUD_GITHUB_ACCOUNT_ID = 'github-default'

export function useCloudGitHubPlugin(isCloudActive: boolean, isViewer: boolean) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const authRefresh = useCloudGitHubAuthRefresh()
  const statusQuery = useCloudGitHubStatus(isCloudActive, authRefresh.polling)
  const prevStateRef = useRef<{ linked: boolean; count: number } | null>(null)

  useEffect(() => {
    if (!isCloudActive) {
      prevStateRef.current = null
      authRefresh.stop()
    }
  }, [authRefresh, isCloudActive])

  useEffect(() => {
    const data = statusQuery.data
    if (!data) return
    const next = { linked: data.user_linked, count: data.installations.length }
    const prev = prevStateRef.current
    prevStateRef.current = next
    if (prev && authRefresh.polling && ((!prev.linked && next.linked) || next.count > prev.count)) {
      authRefresh.stop()
    }
  }, [authRefresh, statusQuery.data])

  const oauth = useCloudGitHubOAuth({
    onStarted: () => {
      authRefresh.start()
      toast('Finish GitHub sign-in in your browser')
    },
    onError: (err) => {
      toast(err.message || 'Could not start GitHub sign-in', 'danger')
    },
  })

  const disconnectMutation = useMutation({
    mutationFn: () => disconnectCloudGitHubAccount(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
      void queryClient.invalidateQueries({ queryKey: keys.plugins })
      toast('Disconnected GitHub account')
    },
    onError: (error: Error) => toast(error.message || 'Could not disconnect GitHub account', 'danger'),
  })

  const plugin = useMemo(
    () =>
      isCloudActive
        ? buildCloudGitHubPlugin({
            status: statusQuery.data,
            authExpired: statusQuery.isError && isCloudAuthExpiredError(statusQuery.error),
            isViewer,
          })
        : null,
    [isCloudActive, isViewer, statusQuery.data, statusQuery.error, statusQuery.isError],
  )

  return {
    id: CLOUD_GITHUB_PLUGIN_ID,
    plugin,
    statusQuery,
    canQuickConnect: Boolean(isCloudActive && !isViewer && statusQuery.data?.configured && !statusQuery.data.user_linked),
    connecting: oauth.isPending,
    disconnecting: disconnectMutation.isPending,
    connect: () => {
      if (isCloudActive && !isViewer) oauth.mutate(undefined)
    },
    disconnect: () => {
      if (isCloudActive && !isViewer) disconnectMutation.mutate()
    },
    startInstallRefresh: authRefresh.start,
  }
}
