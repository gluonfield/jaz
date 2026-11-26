import { useNavigate } from '@tanstack/react-router'
import { Check, GitBranch, LoaderCircle } from 'lucide-react'
import { useEffect } from 'react'
import { Button } from '@/components/ui/Button'
import { Skeleton } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { PluginAvatar } from '@/components/settings/PluginVisuals'
import { accountsSummary, pluginActionLabel, pluginCanConnect } from '@/components/settings/pluginFormatting'
import { usePluginSignIn } from '@/components/settings/usePluginSignIn'
import { useCloudGitHubOAuth, useCloudGitHubStatus, withGitHubPluginSearch } from '@/lib/api/cloudGitHub'
import { useHasCloud, useIsCloudViewer } from '@/lib/cloudSession'

export function ConnectCard({ plugin: pluginID }: { plugin: string }) {
  const hasCloud = useHasCloud()
  if (hasCloud && pluginID === 'github') {
    return <CloudGitHubConnectCard />
  }
  return <PluginConnectCard plugin={pluginID} />
}

function PluginConnectCard({ plugin: pluginID }: { plugin: string }) {
  const signIn = usePluginSignIn()
  if (signIn.catalog.isPending) return <Skeleton className="my-2 h-15 max-w-md rounded-card" />
  const plugin = signIn.plugins.find((item) => item.id === pluginID)
  if (!plugin) return <p>jaz://plugin/{pluginID}/connect</p>
  const connected = Boolean(plugin.auth?.connected)
  const subtitle = connected ? accountsSummary(plugin) : plugin.description
  return (
    <div className="my-2 flex max-w-md items-center gap-3 rounded-card bg-bg px-3 py-2.5 ring-1 ring-border/70">
      <PluginAvatar plugin={plugin} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{plugin.display_name}</span>
        {subtitle ? (
          <span className="block truncate text-[12px] leading-5 text-ink-2" title={subtitle}>
            {subtitle}
          </span>
        ) : null}
      </span>
      {connected && !plugin.auth?.multi_account ? (
        <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-ok">
          <Check size={13} aria-hidden />
          Connected
        </span>
      ) : (
        <Button
          variant="primary"
          size="sm"
          disabled={!pluginCanConnect(plugin) || signIn.isConnecting}
          onClick={() => signIn.start(plugin)}
        >
          {pluginActionLabel(plugin, signIn.isConnecting)}
        </Button>
      )}
      <ConnectionQRModal {...signIn.qrModal} />
    </div>
  )
}

function CloudGitHubConnectCard() {
  const isViewer = useIsCloudViewer()
  const navigate = useNavigate()
  const toast = useToast()
  const oauth = useCloudGitHubOAuth({
    onStarted: () => toast('Finish GitHub sign-in in your browser'),
    onError: (err) => toast(err.message || 'Could not start GitHub sign-in', 'danger'),
  })
  const statusQuery = useCloudGitHubStatus(true, oauth.polling)
  const connected = Boolean(statusQuery.data?.user_linked)
  const openGitHubPlugin = () => void navigate({ to: '.', search: withGitHubPluginSearch })

  useEffect(() => {
    if (oauth.polling && connected) oauth.stopPolling()
  }, [connected, oauth])

  if (statusQuery.isPending) return <Skeleton className="my-2 h-15 max-w-md rounded-card" />
  const status = statusQuery.data
  const configured = Boolean(status?.configured)
  const subtitle = connected
    ? status?.github_login
      ? `Connected as @${status.github_login}`
      : 'Connected to GitHub'
    : !configured
      ? 'GitHub is not configured for this Jaz Cloud deployment yet.'
      : 'Link your GitHub account to browse repositories and open pull requests.'

  return (
    <div className="my-2 flex max-w-md items-center gap-3 rounded-card bg-bg px-3 py-2.5 ring-1 ring-border/70">
      <button
        type="button"
        onClick={openGitHubPlugin}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 text-left"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[8px] bg-surface text-ink">
          <GitBranch size={16} aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium text-ink">GitHub</span>
          <span className="block truncate text-[12px] leading-5 text-ink-2" title={subtitle}>
            {subtitle}
          </span>
        </span>
      </button>
      {connected ? (
        <div className="flex shrink-0 items-center gap-1.5">
          <span className="inline-flex items-center gap-1 text-[12px] text-ok">
            <Check size={13} aria-hidden />
            Connected
          </span>
          <Button variant="ghost" size="sm" onClick={openGitHubPlugin}>
            Manage
          </Button>
        </div>
      ) : !configured ? (
        <Button variant="ghost" size="sm" onClick={openGitHubPlugin}>
          Open in Plugins
        </Button>
      ) : isViewer ? (
        <span className="shrink-0 text-[12px] text-ink-3">View-only</span>
      ) : (
        <Button
          variant="primary"
          size="sm"
          disabled={oauth.isPending}
          onClick={() => oauth.mutate(undefined)}
        >
          {oauth.isPending ? <LoaderCircle size={13} className="animate-spin" /> : null}
          Connect
        </Button>
      )}
    </div>
  )
}
