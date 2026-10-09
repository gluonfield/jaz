import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import '@fontsource/instrument-serif/400-italic.css'
import '@xterm/xterm/css/xterm.css'
import './styles/globals.css'

import { QueryClientProvider } from '@tanstack/react-query'
import { createHashHistory, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ThreadNotifications } from '@/components/notifications/ThreadNotifications'
import { VoiceOverlay } from '@/components/session/VoiceOverlay'
import { PermissionGuide } from '@/components/settings/PermissionGuide'
import { installFileDropGuard } from './components/ui/FileDrop'
import { installKeyboardFocus } from '@/lib/dom/keyboardFocus'
import { clientRuntime } from './lib/clientRuntime'
import { queryClient } from './lib/query/queryClient'
import { routeTree } from './routeTree.gen'
import { App } from './App'
import { telemetry } from './lib/telemetry'
// Side-effect import: applies saved appearance prefs (effects, zoom, fonts) to
// the document root at startup, keeping it aligned with the pre-paint script.
import './lib/appearance'

// Without this, a file dropped outside a drop zone navigates the window to
// its file:// URL, replacing the app shell.
installFileDropGuard()
installKeyboardFocus()

// One open event per launch from the main window — board/widget popouts are
// secondary surfaces and would inflate the count. PostHog derives new vs.
// returning users from the per-install distinct id.
if (clientRuntime.windowKind === 'main') telemetry.appOpened()

if (clientRuntime.windowKind === 'launcher' || clientRuntime.windowKind === 'voice' || clientRuntime.windowKind === 'permission') {
  document.documentElement.classList.add('launcher')
  // zoom 1 keeps drag coordinates 1:1 with screen pixels for region capture.
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

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {clientRuntime.windowKind === 'main' ? <ThreadNotifications /> : null}
      {clientRuntime.windowKind === 'voice' ? <VoiceOverlay /> : clientRuntime.windowKind === 'permission' ? <PermissionGuide /> : <App router={router} />}
    </QueryClientProvider>
  </StrictMode>,
)
