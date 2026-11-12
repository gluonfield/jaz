import { expect, test } from 'bun:test'
import { checkLocalBackend } from '../../scripts/test-local-backend.mjs'

test('local chat, auth, mutations and streams remain available offline through the app-only transport', async () => {
  expect(await checkLocalBackend()).toEqual({ offlineChat: true, recovery: true, isolated: true, visibleWindows: 0, focusedWindows: 0 })
}, 45_000)
