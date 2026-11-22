import { CheckCircle2, LoaderCircle } from 'lucide-react'
import { useMemo } from 'react'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { PluginAvatar } from '@/components/settings/PluginVisuals'
import { accountAddress, pluginActionLabel, pluginCanConnect } from '@/components/settings/pluginFormatting'
import { usePluginSignIn } from '@/components/settings/usePluginSignIn'
import { Button } from '@/components/ui/Button'
import { SkeletonRows } from '@/components/ui/Skeleton'
import type { Plugin } from '@/lib/api/plugins'

const PLUGIN_PRIORITY = ['telegram', 'whatsapp', 'gmail']

export function ConnectionsList() {
  const signIn = usePluginSignIn()
  const sorted = useMemo(
    () => orderPlugins(signIn.plugins.filter((plugin) => plugin.origin === 'builtin')),
    [signIn.plugins],
  )

  return (
    <div>
      {signIn.catalog.isPending ? (
        <SkeletonRows count={3} />
      ) : signIn.catalog.isError ? (
        <p className="text-center text-[13px] text-danger">{signIn.catalog.error.message}</p>
      ) : sorted.length === 0 ? (
        <p className="rounded-[12px] bg-surface px-3 py-3 text-center text-[13px] text-ink-3">
          No integrations are available on this backend yet.
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-1.5">
          {sorted.map((plugin) => (
            <ConnectionRow
              key={plugin.id}
              plugin={plugin}
              connecting={signIn.isConnecting && signIn.connectingPluginID === plugin.id}
              onConnect={() => signIn.start(plugin)}
            />
          ))}
        </div>
      )}
      <p className="mt-3 text-center text-[12px] text-ink-3">Optional — manage all plugins anytime in Settings.</p>

      <ConnectionQRModal {...signIn.qrModal} />
    </div>
  )
}

function ConnectionRow({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: Plugin
  connecting: boolean
  onConnect: () => void
}) {
  const connected = Boolean(plugin.auth?.connected)
  const accounts = (plugin.auth?.accounts ?? []).map(accountAddress).filter(Boolean)
  const showConnect = pluginCanConnect(plugin) && !connected

  return (
    <div className="flex min-w-0 items-center gap-2.5 rounded-[12px] bg-surface px-3 py-2.5">
      <PluginAvatar plugin={plugin} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <p className="truncate text-[13.5px] font-medium text-ink">{plugin.display_name}</p>
          {connected ? <CheckCircle2 size={15} className="shrink-0 text-primary" /> : null}
        </div>
        {connected && accounts.length > 0 ? (
          <p className="truncate text-[12px] text-ink-3">{accounts.join(', ')}</p>
        ) : null}
      </div>
      {showConnect ? (
        <Button size="sm" variant="primary" disabled={connecting} onClick={onConnect}>
          {connecting ? <LoaderCircle size={13} className="animate-spin" /> : null}
          {pluginActionLabel(plugin, connecting)}
        </Button>
      ) : null}
    </div>
  )
}

function orderPlugins(plugins: Plugin[]): Plugin[] {
  const rank = new Map(PLUGIN_PRIORITY.map((id, index) => [id, index]))
  return [...plugins].sort((a, b) => {
    const left = rank.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const right = rank.get(b.id) ?? Number.MAX_SAFE_INTEGER
    if (left !== right) return left - right
    return a.display_name.localeCompare(b.display_name)
  })
}
