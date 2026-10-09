import { Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Skeleton } from '@/components/ui/Skeleton'
import { PluginIcon } from '@/components/settings/ConnectionPluginVisuals'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { accountLabel, pluginActionLabel, pluginCanConnect } from '@/components/settings/connectionFormatting'
import { useConnectionSignIn } from '@/components/settings/useConnectionSignIn'

// The card for a jaz://connect/<plugin> link. Status is read live, so a card
// written before sign-in shows the connected accounts afterwards.
export function ConnectCard({ plugin: pluginID }: { plugin: string }) {
  const signIn = useConnectionSignIn()
  if (signIn.plugins.isPending) return <Skeleton className="my-2 h-15 max-w-md rounded-card" />
  const plugin = signIn.plugins.data?.find((item) => item.id === pluginID)
  if (!plugin) return <p>jaz://connect/{pluginID}</p>
  const connected = plugin.connection?.status === 'connected'
  const subtitle = connected
    ? plugin.connection?.accounts?.map(accountLabel).filter((label) => label !== plugin.name).join(', ')
    : plugin.description
  return (
    <div className="my-2 flex max-w-md items-center gap-3 rounded-card bg-bg px-3 py-2.5 ring-1 ring-border/70">
      <PluginIcon plugin={plugin} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium text-ink">{plugin.name}</span>
        {subtitle ? (
          <span className="block truncate text-[12px] leading-5 text-ink-2" title={subtitle}>
            {subtitle}
          </span>
        ) : null}
      </span>
      {connected && !plugin.multi_account ? (
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
