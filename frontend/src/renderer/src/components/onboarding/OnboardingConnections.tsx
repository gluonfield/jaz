import { CheckCircle2, LoaderCircle } from 'lucide-react'
import { useMemo } from 'react'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { PluginIcon } from '@/components/settings/ConnectionPluginVisuals'
import { accountAddress, pluginActionLabel, pluginCanConnect, pluginInternal } from '@/components/settings/connectionFormatting'
import { useConnectionSignIn } from '@/components/settings/useConnectionSignIn'
import { Button } from '@/components/ui/Button'
import { SkeletonRows } from '@/components/ui/Skeleton'
import type { IntegrationPlugin } from '@/lib/api/types'

// The connections people most want on day one lead the list.
const PLUGIN_PRIORITY = ['telegram', 'whatsapp', 'gmail']

export function ConnectionsList() {
  const signIn = useConnectionSignIn()
  const plugins = signIn.plugins
  const sorted = useMemo(() => orderPlugins(plugins.data ?? []), [plugins.data])

  return (
    <div>
      {plugins.isPending ? (
        <SkeletonRows count={3} />
      ) : plugins.isError ? (
        <p className="text-center text-[13px] text-danger">{plugins.error.message}</p>
      ) : sorted.length === 0 ? (
        <p className="rounded-[12px] bg-surface px-3 py-3 text-center text-[13px] text-ink-3">
          No connections are available on this backend yet.
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
      <p className="mt-3 text-center text-[12px] text-ink-3">Optional — add more anytime in Settings.</p>

      <ConnectionQRModal
        plugin={signIn.activeQR?.plugin}
        qr={signIn.activeQR?.qr}
        status={signIn.qrStatus}
        loading={signIn.qrLoading}
        refreshing={signIn.qrRefreshing}
        passwordSubmitting={signIn.qrPasswordSubmitting}
        onClose={signIn.closeQR}
        onRefresh={signIn.refreshQR}
        onSubmitPassword={signIn.submitQRPassword}
      />
    </div>
  )
}

function ConnectionRow({
  plugin,
  connecting,
  onConnect,
}: {
  plugin: IntegrationPlugin
  connecting: boolean
  onConnect: () => void
}) {
  const connected = plugin.connection?.status === 'connected'
  const accounts = (plugin.connection?.accounts ?? []).map(accountAddress).filter(Boolean)
  const showConnect = pluginCanConnect(plugin) && !connected

  return (
    <div className="flex min-w-0 items-center gap-2.5 rounded-[12px] bg-surface px-3 py-2.5">
      <PluginIcon plugin={plugin} compact />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <p className="truncate text-[13.5px] font-medium text-ink">{plugin.name}</p>
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

function orderPlugins(plugins: IntegrationPlugin[]): IntegrationPlugin[] {
  const rank = new Map(PLUGIN_PRIORITY.map((id, index) => [id, index]))
  return plugins.filter((plugin) => !pluginInternal(plugin)).sort((a, b) => {
    const left = rank.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const right = rank.get(b.id) ?? Number.MAX_SAFE_INTEGER
    if (left !== right) return left - right
    return a.name.localeCompare(b.name)
  })
}
