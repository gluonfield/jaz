import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import './styles/globals.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createHashHistory, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { LaunchScreen } from './components/launch/LaunchScreen'
import { useConnection } from './lib/connection'
import { queryClient } from './lib/query/queryClient'
import { routeTree } from './routeTree.gen'

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

// The app only renders against a healthy backend; otherwise the launch
// screen owns the window (start locally / connect to remote).
function App() {
  const { status } = useConnection()
  if (status !== 'connected') return <LaunchScreen />
  return <RouterProvider router={router} />
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
