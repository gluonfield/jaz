import { describe, expect, test } from 'bun:test'
import { buildAppSandboxDocument, dispatchAppBridgeCall, isAppBridgeRequest } from './appBridge.ts'

describe('window.jaz v2 app bridge', () => {
  test('injects v2 capabilities, storage, ai, tools, and desktop globals into sandbox document', () => {
    const doc = buildAppSandboxDocument(
      'Demo App',
      '<div id="root">Hello</div>',
      ':root{--color-bg:#fff;}',
      ['storage:personal', 'ai:complete', 'tools:call'],
      ['crm_search'],
    )
    expect(doc).toContain('window.jaz = Object.assign')
    expect(doc).toContain('storage:personal')
    expect(doc).toContain('crm_search')
    expect(doc).toContain("default-src 'none'")
  })

  test('detects bridge request messages safely', () => {
    expect(isAppBridgeRequest({ type: 'jaz:app-bridge-request', id: '1', method: 'storage.get' })).toBe(true)
    expect(isAppBridgeRequest({ type: 'other', id: '1', method: 'storage.get' })).toBe(false)
    expect(isAppBridgeRequest(null)).toBe(false)
  })

  test('blocks undeclared capabilities, disallowed tools, and paused desktop permissions', async () => {
    let sharedPromptCalled = false
    const called = []
    const ctx = {
      app: {
        id: 'aaaaaaaaaaaaaaaaaaaaaaaaaa',
        title: 'Strict App',
        capabilities: ['storage:personal', 'storage:shared', 'tools:call'],
        allowed_tools: ['crm_search'],
        bundle_sha256: 'a'.repeat(64),
      },
      ticket: {
        ticket: 'tok',
        expires_at: new Date().toISOString(),
        app_id: 'aaaaaaaaaaaaaaaaaaaaaaaaaa',
        version: 1,
        bundle_sha256: 'a'.repeat(64),
        entry_path: 'index.html',
        content_origin: 'http://127.0.0.1:4242',
        content_url: 'http://127.0.0.1:4242/app-content/aaaaaaaaaaaaaaaaaaaaaaaaaa/index.html?ticket=tok',
      },
      api: {
        getAppKV: async (id, key, scope) => {
          called.push(['get', id, key, scope])
          return { scope, key, value: 'saved', version: 1 }
        },
        putAppKV: async (id, key, value, opts) => {
          called.push(['put', id, key, value, opts])
          return { scope: opts.scope, key, value, version: 2 }
        },
        deleteAppKV: async () => {},
        listAppKV: async () => ({ scope: 'personal', keys: [] }),
        completeAppAI: async () => ({ text: 'ok' }),
        callAppTool: async (id, req) => {
          called.push(['tool', id, req.tool])
          return { content: [{ type: 'text', text: 'result' }] }
        },
      },
      resolveBundleAssetUrl: (rel) => `http://127.0.0.1:4242/app-content/a/${rel}?ticket=tok`,
      confirmSharedWrite: async () => {
        sharedPromptCalled = true
        return false
      },
      desktopPermissions: ['notifications'],
      desktopPermissionsPaused: true,
    }

    await expect(dispatchAppBridgeCall(ctx, 'ai.complete', { prompt: 'hi' })).rejects.toThrow('ai:complete')
    await expect(dispatchAppBridgeCall(ctx, 'tools.call', { tool: 'evil_tool' })).rejects.toThrow('allowed_tools')
    await expect(dispatchAppBridgeCall(ctx, 'storage.set', { key: 'k', value: 'v', shared: true })).rejects.toThrow('cancelled')
    expect(sharedPromptCalled).toBe(true)
    await expect(dispatchAppBridgeCall(ctx, 'desktop.notify', { title: 'Hi', body: 'There' })).rejects.toThrow('not approved')
    await expect(dispatchAppBridgeCall(ctx, 'resolveAsset', { path: '../secret.txt' })).rejects.toThrow('relative')

    const kv = await dispatchAppBridgeCall(ctx, 'storage.get', { key: 'theme' })
    expect(kv).toEqual({ scope: 'personal', key: 'theme', value: 'saved', version: 1 })
    const toolRes = await dispatchAppBridgeCall(ctx, 'tools.call', { tool: 'crm_search', arguments: { q: 'acme' } })
    expect(toolRes).toEqual({ content: [{ type: 'text', text: 'result' }] })
    const asset = await dispatchAppBridgeCall(ctx, 'resolveAsset', { path: 'data/chart.json' })
    expect(asset).toEqual({ url: 'http://127.0.0.1:4242/app-content/a/data/chart.json?ticket=tok' })
    expect(called).toEqual([
      ['get', 'aaaaaaaaaaaaaaaaaaaaaaaaaa', 'theme', 'personal'],
      ['tool', 'aaaaaaaaaaaaaaaaaaaaaaaaaa', 'crm_search'],
    ])
  })
})
