import { ChartNoAxesColumn, Cloud, Laptop, LoaderCircle, LogIn, LogOut, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { CloudIdentityBadge } from '@/components/connection/BackendSwitcherMenu'
import { SettingsCard, SettingsRow } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import { clientRuntime } from '@/lib/clientRuntime'
import { attachCloudSignIn, signOutCloudAccount } from '@/lib/cloudAccount'
import { DEFAULT_CLOUD_URL, useCloudSession } from '@/lib/cloudSession'
import { disconnectBackend, startCloudSignIn } from '@/lib/connection'
import type { SettingsSection } from './sections'

export function CloudSettings({
  onNavigate,
}: {
  onNavigate: (section: SettingsSection) => void
}) {
  const cloud = useCloudSession()
  const toast = useToast()
  const [url, setUrl] = useState(() => cloud.url || clientRuntime.cloudUrl || DEFAULT_CLOUD_URL)
  const [busy, setBusy] = useState(false)
  const [awaitingBrowser, setAwaitingBrowser] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  useEffect(() => {
    if (cloud.url) setUrl(cloud.url)
  }, [cloud.url])

  useEffect(() => {
    if (cloud.active) {
      setAwaitingBrowser(false)
      setFormError(null)
    }
  }, [cloud.active])

  const startSignIn = async (targetUrl: string, loginPath?: string) => {
    const trimmed = targetUrl.trim()
    if (!trimmed) return
    setBusy(true)
    setFormError(null)
    try {
      const err = cloud.source === 'primary'
        ? await startCloudSignIn(trimmed, loginPath)
        : await attachCloudSignIn(trimmed, loginPath)
      if (err) {
        setFormError(err)
        return
      }
      if (clientRuntime.kind === 'electron') {
        setAwaitingBrowser(true)
      }
    } catch (error) {
      setFormError(error instanceof Error ? error.message : 'Could not start Jaz Cloud sign-in')
    } finally {
      setBusy(false)
    }
  }

  const handleSignOut = () => {
    setAwaitingBrowser(false)
    setFormError(null)
    if (cloud.source === 'primary') {
      disconnectBackend()
      return
    }
    signOutCloudAccount()
    toast('Signed out of Jaz Cloud')
  }

  const visibleError = formError || cloud.error

  return (
    <section className="flex flex-col gap-5 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">Jaz Cloud</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">
          {cloud.source === 'primary'
            ? 'Your active Jaz Cloud workspace and organization identity.'
            : 'Attach a Jaz Cloud account alongside your local workspace — keep local threads, bots, boards, and memory.'}
        </p>
      </div>

      {cloud.active ? (
        <>
          <SettingsCard className="overflow-hidden">
            <div className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-3">
              <div className="min-w-0 flex-1">
                {cloud.user ? (
                  <CloudIdentityBadge user={cloud.user} />
                ) : (
                  <div className="flex items-center gap-2 px-1">
                    <Cloud size={15} className="text-primary" />
                    <span className="truncate text-[13px] font-medium text-ink">{cloud.url}</span>
                  </div>
                )}
              </div>
              <span className="inline-flex items-center gap-1.5 rounded-full bg-primary-soft px-2.5 py-0.5 text-[11px] font-medium text-primary">
                <span className="size-1.5 rounded-full bg-ok" />
                {cloud.source === 'primary' ? 'Connected' : 'Attached'}
              </span>
            </div>

            <SettingsRow
              title="Cloud server"
              description={cloud.source === 'attached' ? 'Active alongside This computer.' : 'Primary workspace server.'}
            >
              <span className="font-mono text-[12px] text-ink-2 select-all">{cloud.url}</span>
            </SettingsRow>

            <SettingsRow
              title="Session"
              description={
                cloud.source === 'attached'
                  ? 'Signing out detaches Jaz Cloud without disconnecting your local workspace.'
                  : 'Revoke this cloud session and return to the sign-in screen.'
              }
            >
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={busy}
                  onClick={() => void startSignIn(cloud.url, cloud.oidcLoginUrl || undefined)}
                >
                  {busy ? <LoaderCircle size={13} className="animate-spin" /> : <LogIn size={13} />}
                  Re-authenticate SSO
                </Button>
                <Button size="sm" variant="ghost" disabled={busy} onClick={handleSignOut}>
                  <LogOut size={13} />
                  {cloud.source === 'attached' ? 'Sign out of Jaz Cloud' : 'Sign out'}
                </Button>
              </div>
            </SettingsRow>
          </SettingsCard>

          <div>
            <p className="mb-2 text-[12px] font-medium text-ink-3">Cloud workspace</p>
            <SettingsCard className="overflow-hidden">
              <SettingsRow
                title="Connected Machines"
                description="Pair Mac, Windows, or Linux workstations for folder staging and local execution."
              >
                <Button size="sm" variant="secondary" onClick={() => onNavigate('machines')}>
                  <Laptop size={13} />
                  Manage machines
                </Button>
              </SettingsRow>
              <SettingsRow
                title="Cloud Instructions"
                description="Edit organization, space, and personal AGENTS.md and SOUL.md files."
              >
                <Button size="sm" variant="secondary" onClick={() => onNavigate('personalization')}>
                  <Sparkles size={13} />
                  Open instructions
                </Button>
              </SettingsRow>
              <SettingsRow
                title="Cloud Usage"
                description="View monthly organization and personal cloud spend caps."
              >
                <Button size="sm" variant="secondary" onClick={() => onNavigate('usage')}>
                  <ChartNoAxesColumn size={13} />
                  View usage
                </Button>
              </SettingsRow>
            </SettingsCard>
          </div>
        </>
      ) : (
        <SettingsCard className="p-4">
          <form
            onSubmit={(event) => {
              event.preventDefault()
              void startSignIn(url)
            }}
            className="flex flex-col gap-3"
          >
            <div className="flex items-center gap-2">
              <Cloud size={15} className="shrink-0 text-primary" />
              <p className="text-[13.5px] font-medium text-ink">Connect Jaz Cloud</p>
            </div>
            <p className="text-[12.5px] text-ink-2">
              Enter your Jaz Cloud HTTPS address and sign in with SSO. Cloud threads and Spaces will appear right alongside your local threads.
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                value={url}
                onChange={(event) => {
                  setUrl(event.target.value)
                  if (formError) setFormError(null)
                }}
                placeholder={DEFAULT_CLOUD_URL}
                aria-label="Jaz Cloud server URL"
                spellCheck={false}
                disabled={busy}
                className="h-9 min-w-[240px] flex-1 font-mono text-[12.5px]"
              />
              <Button type="submit" variant="primary" disabled={busy || !url.trim()}>
                {busy || cloud.status === 'checking' ? (
                  <LoaderCircle size={14} className="animate-spin" />
                ) : (
                  <LogIn size={14} />
                )}
                {awaitingBrowser ? 'Reopen browser sign-in' : 'Sign in with SSO'}
              </Button>
            </div>
            {awaitingBrowser ? (
              <p className="text-[12px] text-ink-3">
                Complete sign-in in your browser — Jaz Cloud will attach automatically.
              </p>
            ) : null}
            {visibleError ? (
              <p role="alert" className="text-[12px] text-danger">
                {visibleError}
              </p>
            ) : null}
          </form>
        </SettingsCard>
      )}
    </section>
  )
}
