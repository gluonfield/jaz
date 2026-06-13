import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import '@fontsource/instrument-serif/400-italic.css'
import '@xterm/xterm/css/xterm.css'
import './styles/globals.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createHashHistory, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { LaunchScreen, ReconnectingBanner } from './components/launch/LaunchScreen'
import { installFileDropGuard } from './components/ui/FileDrop'
import { useConnection } from './lib/connection'
import { queryClient } from './lib/query/queryClient'
import { routeTree } from './routeTree.gen'

// Without this, a file dropped outside a drop zone navigates the window to
// its file:// URL, replacing the app shell.
installFileDropGuard()

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
  if (status === 'connected' || status === 'reconnecting') {
    return (
      <>
        <RouterProvider router={router} />
        <ReconnectingBanner show={status === 'reconnecting'} />
      </>
    )
  }
  return <LaunchScreen />
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
