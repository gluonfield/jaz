import { Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { PluginIcon } from '@/components/settings/ConnectionPluginVisuals'
import { ConnectionQRModal } from '@/components/settings/ConnectionQRModal'
import { accountLabel, pluginActionLabel, pluginCanConnect } from '@/components/settings/connectionFormatting'
import { useConnectionSignIn } from '@/components/settings/useConnectionSignIn'

// The card for a jaz://connect/<plugin> link. Status is read live, so a card
// written before sign-in shows the connected accounts afterwards.
export function ConnectCard({ plugin: pluginID }: { plugin: string }) {
  const signIn = useConnectionSignIn()
  const plugin = signIn.plugins.data?.find((item) => item.id === pluginID)
  if (!plugin) return <p>jaz://connect/{pluginID}</p>
  const accounts = plugin.connection?.accounts ?? []
  const subtitle = accounts.length
    ? accounts.map(accountLabel).filter((label) => label !== plugin.name).join(', ')
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
      {accounts.length && !plugin.multi_account ? (
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
          {accounts.length && !signIn.isConnecting ? 'Add account' : pluginActionLabel(plugin, signIn.isConnecting)}
        </Button>
      )}
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
