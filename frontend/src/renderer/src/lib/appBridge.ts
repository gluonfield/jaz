import { buildArtifactDocument } from '@/lib/artifacts'
import type {
  AppTicketResponse,
  JazApp,
  callAppTool as CallAppToolFn,
  completeAppAI as CompleteAppAIFn,
  deleteAppKV as DeleteAppKVFn,
  getAppKV as GetAppKVFn,
  listAppKV as ListAppKVFn,
  putAppKV as PutAppKVFn,
} from '@/lib/api/apps'
import type { DesktopAppsAPI } from '@shared/desktopApps'

export interface AppBridgeAPI {
  callAppTool: typeof CallAppToolFn
  completeAppAI: typeof CompleteAppAIFn
  deleteAppKV: typeof DeleteAppKVFn
  getAppKV: typeof GetAppKVFn
  listAppKV: typeof ListAppKVFn
  putAppKV: typeof PutAppKVFn
}

export interface AppBridgeContext {
  app: Pick<JazApp, 'id' | 'title' | 'capabilities' | 'allowed_tools' | 'bundle_sha256'>
  ticket: AppTicketResponse
  resolveBundleAssetUrl: (relpath: string) => string
  confirmSharedWrite: () => Promise<boolean>
  api?: AppBridgeAPI
  desktopApps?: DesktopAppsAPI
  desktopPermissions?: string[]
  desktopPermissionsPaused?: boolean
  onOpenLink?: (href: string) => boolean
  onError?: (message: string) => void
}

const APP_BRIDGE_V2_SCRIPT = (capabilities: readonly string[], allowedTools: readonly string[]) => `
(() => {
  let nextReqId = 1;
  const pending = new Map();
  window.addEventListener('message', (event) => {
    if (event.source !== parent) return;
    const msg = event.data;
    if (msg?.type !== 'jaz:app-bridge-response' || typeof msg.id !== 'string') return;
    const entry = pending.get(msg.id);
    if (!entry) return;
    pending.delete(msg.id);
    if (msg.error) entry.reject(new Error(String(msg.error)));
    else entry.resolve(msg.result);
  });
  const callHost = (method, params) => {
    const id = 'jaz-app-' + (nextReqId++);
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject });
      parent.postMessage({ type: 'jaz:app-bridge-request', id, method, params: params || {} }, '*');
    });
  };
  const storage = {
    get: (key, opts) => callHost('storage.get', { key, shared: Boolean(opts && opts.shared) }),
    set: (key, value, opts) => callHost('storage.set', {
      key,
      value: typeof value === 'string' ? value : JSON.stringify(value),
      shared: Boolean(opts && opts.shared),
      expectedRevision: opts && typeof opts.expectedRevision === 'number' ? opts.expectedRevision : undefined,
    }),
    delete: (key, opts) => callHost('storage.delete', {
      key,
      shared: Boolean(opts && opts.shared),
      expectedRevision: opts && typeof opts.expectedRevision === 'number' ? opts.expectedRevision : undefined,
    }),
    list: (opts) => callHost('storage.list', {
      prefix: opts && typeof opts.prefix === 'string' ? opts.prefix : '',
      shared: Boolean(opts && opts.shared),
    }),
  };
  const ai = {
    complete: (input) => {
      const payload = typeof input === 'string' ? { prompt: input } : (input || {});
      return callHost('ai.complete', payload);
    },
  };
  const tools = {
    call: (tool, args, alias) => callHost('tools.call', { tool, arguments: args || {}, alias: alias || '' }),
  };
  const desktop = {
    notify: (title, body) => callHost('desktop.notify', { title, body }),
    setAlwaysOnTop: (enabled) => callHost('desktop.setAlwaysOnTop', { enabled: Boolean(enabled) }),
    fs: {
      pickFolder: () => callHost('desktop.fs.pickFolder', {}),
      list: (folder, relPath) => callHost('desktop.fs.list', { folder, relPath: relPath || '' }),
      read: (folder, relPath) => callHost('desktop.fs.read', { folder, relPath }),
      write: (folder, relPath, content) => callHost('desktop.fs.write', { folder, relPath, content }),
    },
  };
  const resolveAsset = (path) => callHost('resolveAsset', { path });
  window.jaz = Object.assign(window.jaz || {}, {
    version: 2,
    capabilities: ${JSON.stringify(capabilities)},
    allowedTools: ${JSON.stringify(allowedTools)},
    storage,
    ai,
    tools,
    desktop,
    resolveAsset,
  });
})();
`

export function buildAppSandboxDocument(
  title: string,
  code: string,
  themeCSS: string,
  capabilities: readonly string[],
  allowedTools: readonly string[],
  opts: { measureLayout?: boolean } = {},
): string {
  const base = buildArtifactDocument(
    { title, code, loadingMessages: [] },
    themeCSS,
    { measureLayout: opts.measureLayout },
  )
  const scriptTag = `<script>${APP_BRIDGE_V2_SCRIPT(capabilities, allowedTools)}</script>`
  const headClose = base.indexOf('</head>')
  if (headClose !== -1) {
    return base.slice(0, headClose) + scriptTag + base.slice(headClose)
  }
  return scriptTag + base
}

export interface AppBridgeRequestMessage {
  type: 'jaz:app-bridge-request'
  id: string
  method: string
  params?: Record<string, unknown>
}

export function isAppBridgeRequest(data: unknown): data is AppBridgeRequestMessage {
  if (!data || typeof data !== 'object') return false
  const rec = data as Record<string, unknown>
  return rec.type === 'jaz:app-bridge-request' && typeof rec.id === 'string' && typeof rec.method === 'string'
}

async function loadApi(ctx: AppBridgeContext): Promise<AppBridgeAPI> {
  return ctx.api ?? (await import('@/lib/api/apps'))
}

export async function dispatchAppBridgeCall(
  ctx: AppBridgeContext,
  method: string,
  params: Record<string, unknown> = {},
): Promise<unknown> {
  const caps = new Set(ctx.app.capabilities ?? [])
  const desktopAllowed = (perm: string): boolean => {
    if (!ctx.desktopApps || ctx.desktopPermissionsPaused) return false
    return (ctx.desktopPermissions ?? []).includes(perm)
  }

  switch (method) {
    case 'storage.get': {
      const key = typeof params.key === 'string' ? params.key : ''
      const scope = params.shared ? 'shared' : 'personal'
      const reqCap = scope === 'shared' ? 'storage:shared' : 'storage:personal'
      if (!caps.has(reqCap)) throw new Error(`App does not declare ${reqCap}`)
      return (await loadApi(ctx)).getAppKV(ctx.app.id, key, scope)
    }
    case 'storage.set': {
      const key = typeof params.key === 'string' ? params.key : ''
      const value = typeof params.value === 'string' ? params.value : JSON.stringify(params.value ?? '')
      const scope = params.shared ? 'shared' : 'personal'
      const reqCap = scope === 'shared' ? 'storage:shared' : 'storage:personal'
      if (!caps.has(reqCap)) throw new Error(`App does not declare ${reqCap}`)
      if (scope === 'shared') {
        const ok = await ctx.confirmSharedWrite()
        if (!ok) throw new Error('Shared storage write cancelled by user')
      }
      const expectedRevision = typeof params.expectedRevision === 'number' ? params.expectedRevision : undefined
      return (await loadApi(ctx)).putAppKV(ctx.app.id, key, value, { scope, expectedRevision })
    }
    case 'storage.delete': {
      const key = typeof params.key === 'string' ? params.key : ''
      const scope = params.shared ? 'shared' : 'personal'
      const reqCap = scope === 'shared' ? 'storage:shared' : 'storage:personal'
      if (!caps.has(reqCap)) throw new Error(`App does not declare ${reqCap}`)
      if (scope === 'shared') {
        const ok = await ctx.confirmSharedWrite()
        if (!ok) throw new Error('Shared storage delete cancelled by user')
      }
      const expectedRevision = typeof params.expectedRevision === 'number' ? params.expectedRevision : undefined
      return (await loadApi(ctx)).deleteAppKV(ctx.app.id, key, { scope, expectedRevision })
    }
    case 'storage.list': {
      const prefix = typeof params.prefix === 'string' ? params.prefix : ''
      const scope = params.shared ? 'shared' : 'personal'
      const reqCap = scope === 'shared' ? 'storage:shared' : 'storage:personal'
      if (!caps.has(reqCap)) throw new Error(`App does not declare ${reqCap}`)
      return (await loadApi(ctx)).listAppKV(ctx.app.id, { scope, prefix })
    }
    case 'ai.complete': {
      if (!caps.has('ai:complete')) throw new Error('App does not declare ai:complete')
      const prompt = typeof params.prompt === 'string' ? params.prompt : ''
      const system = typeof params.system === 'string' ? params.system : undefined
      const model = typeof params.model === 'string' ? params.model : undefined
      const maxTokens = typeof params.maxTokens === 'number' ? params.maxTokens : undefined
      return (await loadApi(ctx)).completeAppAI(ctx.app.id, { prompt, system, model, max_tokens: maxTokens })
    }
    case 'tools.call': {
      if (!caps.has('tools:call')) throw new Error('App does not declare tools:call')
      const tool = typeof params.tool === 'string' ? params.tool.trim() : ''
      if (!tool || !(ctx.app.allowed_tools ?? []).includes(tool)) {
        throw new Error(`Tool "${tool}" is not in this app's allowed_tools list`)
      }
      const args = (params.arguments && typeof params.arguments === 'object' ? params.arguments : {}) as Record<string, unknown>
      const alias = typeof params.alias === 'string' ? params.alias : undefined
      return (await loadApi(ctx)).callAppTool(ctx.app.id, { tool, arguments: args, alias })
    }
    case 'resolveAsset': {
      const relpath = typeof params.path === 'string' ? params.path.trim() : ''
      if (!relpath || relpath.startsWith('/') || relpath.includes('..') || relpath.includes('\\')) {
        throw new Error('Asset path must be relative to the app bundle')
      }
      return { url: ctx.resolveBundleAssetUrl(relpath) }
    }
    case 'desktop.notify': {
      if (!desktopAllowed('notifications')) throw new Error('Desktop notifications permission not approved for this app version')
      return ctx.desktopApps!.showNotification(
        ctx.app.id,
        typeof params.title === 'string' ? params.title : ctx.app.title,
        typeof params.body === 'string' ? params.body : '',
      )
    }
    case 'desktop.setAlwaysOnTop': {
      if (!desktopAllowed('always_on_top')) throw new Error('Always-on-top permission not approved for this app version')
      return ctx.desktopApps!.setAlwaysOnTop(ctx.app.id, Boolean(params.enabled))
    }
    case 'desktop.fs.pickFolder': {
      if (!desktopAllowed('fs')) throw new Error('Scoped folder permission not approved for this app version')
      return ctx.desktopApps!.pickScopedFolder(ctx.app.id)
    }
    case 'desktop.fs.list': {
      if (!desktopAllowed('fs')) throw new Error('Scoped folder permission not approved for this app version')
      const folder = typeof params.folder === 'string' ? params.folder : ''
      const relPath = typeof params.relPath === 'string' ? params.relPath : ''
      return ctx.desktopApps!.fsList(ctx.app.id, folder, relPath)
    }
    case 'desktop.fs.read': {
      if (!desktopAllowed('fs')) throw new Error('Scoped folder permission not approved for this app version')
      const folder = typeof params.folder === 'string' ? params.folder : ''
      const relPath = typeof params.relPath === 'string' ? params.relPath : ''
      return ctx.desktopApps!.fsRead(ctx.app.id, folder, relPath)
    }
    case 'desktop.fs.write': {
      if (!desktopAllowed('fs')) throw new Error('Scoped folder permission not approved for this app version')
      const folder = typeof params.folder === 'string' ? params.folder : ''
      const relPath = typeof params.relPath === 'string' ? params.relPath : ''
      const content = typeof params.content === 'string' ? params.content : ''
      return ctx.desktopApps!.fsWrite(ctx.app.id, folder, relPath, content)
    }
    default:
      throw new Error(`Unknown window.jaz bridge method: ${method}`)
  }
}
