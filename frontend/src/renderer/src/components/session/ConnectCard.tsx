import { Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Skeleton } from '@/components/ui/Skeleton'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { PluginAvatar } from '@/components/settings/PluginVisuals'
import { accountsSummary, pluginActionLabel, pluginCanConnect } from '@/components/settings/pluginFormatting'
import { usePluginSignIn } from '@/components/settings/usePluginSignIn'

export function ConnectCard({ plugin: pluginID }: { plugin: string }) {
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
