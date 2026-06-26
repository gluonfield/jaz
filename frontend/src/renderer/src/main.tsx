import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import '@fontsource/instrument-serif/400-italic.css'
import '@xterm/xterm/css/xterm.css'
import './styles/globals.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createHashHistory, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BackendTransition } from './components/connection/BackendTransition'
import { LaunchScreen, ReconnectingBanner } from './components/launch/LaunchScreen'
import { OnboardingGate } from './components/onboarding/OnboardingGate'
import { installFileDropGuard } from './components/ui/FileDrop'
import { useBackendChange, useConnection } from './lib/connection'
import { clientRuntime } from './lib/clientRuntime'
import { queryClient } from './lib/query/queryClient'
import { routeTree } from './routeTree.gen'
import { telemetry } from './lib/telemetry'
// Side-effect import: applies saved appearance prefs (effects, zoom, fonts) to
// the document root at startup, keeping it aligned with the pre-paint script.
import './lib/appearance'

// Without this, a file dropped outside a drop zone navigates the window to
// its file:// URL, replacing the app shell.
installFileDropGuard()

// One open event per launch from the main window — board/widget popouts are
// secondary surfaces and would inflate the count. PostHog derives new vs.
// returning users from the per-install distinct id.
if (clientRuntime.windowKind === 'main') telemetry.appOpened()

// The launcher window floats over other apps: keep its page see-through so only
// the card paints, and pin zoom to 1 so drag coordinates map 1:1 to screen
// pixels for region capture (it never shows the appearance font-scale anyway).
if (clientRuntime.windowKind === 'launcher') {
  document.documentElement.classList.add('launcher')
  document.documentElement.style.zoom = '1'
}

const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  // Packaged builds load the renderer from file://, where pathname-based
  // history can never match a route; hash history works in both.
  history: window.location.protocol === 'file:' ? createHashHistory() : undefined,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// The app renders while connected — and stays mounted through transient
// losses ('reconnecting', banner over live UI) so drafts and streams survive
// a blip. Only a sustained outage hands the window to the launch screen.
function App() {
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

  const connected = status === 'connected' || status === 'reconnecting'
  const app = <RouterProvider router={router} />
  return (
    <>
      {/* plays over everything whenever the backend changes */}
      <BackendTransition />
      {connected ? (
        <>
          {clientRuntime.windowKind === 'main' ? <OnboardingGate>{app}</OnboardingGate> : app}
          <ReconnectingBanner show={status === 'reconnecting'} />
        </>
      ) : (
        <LaunchScreen />
      )}
    </>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
