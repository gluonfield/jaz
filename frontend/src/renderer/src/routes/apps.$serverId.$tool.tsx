import { createFileRoute } from '@tanstack/react-router'

// The app itself lives in MCPApps over the content card, which stays mounted
// across sections; this route only makes one sidebar entrypoint the active one
// and carries to it a deep link's app-relative path or a new call's input.
export const Route = createFileRoute('/apps/$serverId/$tool')({
  validateSearch: (search): { path?: string; input?: Record<string, unknown> } => ({
    ...(typeof search.path === 'string' && search.path.startsWith('/') && { path: search.path }),
    ...(typeof search.input === 'object' && search.input !== null && !Array.isArray(search.input) && { input: search.input as Record<string, unknown> }),
  }),
})
