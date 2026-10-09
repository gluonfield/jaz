import { RouterProvider, type AnyRouter } from '@tanstack/react-router'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { BackendTransition } from '@/components/connection/BackendTransition'
import { LaunchScreen, ReconnectingBanner } from '@/components/launch/LaunchScreen'
import { OnboardingGate } from '@/components/onboarding/OnboardingGate'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { clientRuntime } from '@/lib/clientRuntime'
import { useBackendChange, useConnection } from '@/lib/connection'
import { devPreview } from '@/lib/devPreview'

// The app renders while connected — and stays mounted through transient
// losses ('reconnecting', banner over live UI) so drafts and streams survive
// a blip. Only a sustained outage hands the window to the launch screen.
export function App({ router }: { router: AnyRouter }) {
  const { status } = useConnection()
  // Reset to home whenever the backend changes — done here, above the router and
  // the onboarding gate, so it also covers landing after a fresh backend's
  // onboarding finishes (the router mounts onto this location). The persisted
  // route otherwise points at the previous backend's data — a thread/board/loop
  // id the new backend doesn't have — which 404s. The launcher window lives at
  // one route, so never yank it to home.
  useBackendChange(() => {
    if (clientRuntime.windowKind !== 'launcher') router.history.push('/')
  })

  const connected = devPreview('launch') === null && (status === 'connected' || status === 'reconnecting')
  const app = <RouterProvider router={router} />
  // Agents keep browsing through an outage, so their browsers and panels
  // outlive the launch screen; only a backend switch clears them.
  return (
    <>
      {/* plays over everything whenever the backend changes */}
      <BackendTransition />
      <BrowserWorkspace>
        <SidePanelStateProvider>
          {connected ? (
            <>
              {clientRuntime.windowKind === 'main' ? <OnboardingGate>{app}</OnboardingGate> : app}
              <ReconnectingBanner show={status === 'reconnecting'} />
            </>
          ) : (
            <LaunchScreen />
          )}
        </SidePanelStateProvider>
      </BrowserWorkspace>
    </>
  )
}
