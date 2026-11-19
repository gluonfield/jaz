import { Button } from '@/components/ui/Button'
import { clientRuntime, openExternal } from '@/lib/clientRuntime'

export function GooglePasskeyNotice({ sourceUrl, title }: { sourceUrl: string; title?: string }) {
  if (!clientRuntime.capabilities.browserPasskeyFallback) return null
  const url = URL.parse(sourceUrl)
  if (url?.origin !== 'https://accounts.google.com' || !/^\/v3\/signin\/challenge\/pk(?:\/|$)/.test(url.pathname)) return null
  const destination = title === 'Google Cloud Platform' ? 'https://console.cloud.google.com/' : 'https://accounts.google.com/'
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b border-border px-3 py-1.5">
      <p className="text-xs text-ink-2">Use your default browser to sign in with a passkey. This starts a separate session.</p>
      <Button size="sm" variant="secondary" onClick={() => openExternal(destination)}>Open Google sign-in in browser</Button>
    </div>
  )
}
