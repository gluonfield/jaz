import type { ArtifactEvent } from './api/types'

export interface ArtifactInput {
  title: string
  code: string
  loadingMessages: string[]
}

export interface ResolvedAsset {
  url: string
  size: number
  contentType: string
  etag?: string
  expiresAt: string
}

const RESIZE_OBSERVER_LAYOUT_WARNING = 'ResizeObserver loop completed with undelivered notifications.'

export function artifactErrorForDisplay(message: string): string | undefined {
  return message === RESIZE_OBSERVER_LAYOUT_WARNING ? undefined : message
}

const ARTIFACT_CSP =
  "default-src 'none'; script-src 'unsafe-inline' https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com; style-src 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; font-src data: https://fonts.gstatic.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; img-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://fonts.gstatic.com https://fonts.googleapis.com; connect-src https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com; media-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; frame-src 'none'; base-uri 'none'; form-action 'none'"

export function sanitizeAssetOrigin(raw?: string): string {
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    if (parsed.protocol === 'jaz-local:') {
      return parsed.host === 'backend' ? 'jaz-local://backend' : ''
    }
    if (parsed.protocol === 'https:') {
      return parsed.origin
    }
    if (parsed.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname)) {
      return parsed.origin
    }
  } catch {
    // Ignore malformed origins.
  }
  return ''
}

export function buildArtifactCSP(pluginView = false, assetOrigin?: string): string {
  if (!pluginView) return ARTIFACT_CSP
  const safeOrigin = sanitizeAssetOrigin(assetOrigin)
  const originSuffix = safeOrigin ? ` ${safeOrigin}` : ''
  return [
    "default-src 'none'",
    "script-src 'unsafe-inline' 'wasm-unsafe-eval' https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com",
    "style-src 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com",
    'font-src data: https://fonts.gstatic.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com',
    `img-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://fonts.gstatic.com https://fonts.googleapis.com${originSuffix}`,
    `connect-src https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com${originSuffix}`,
    `media-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com${originSuffix}`,
    'worker-src blob:',
    "frame-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join('; ')
}

export function resolvePluginAssetRelpath(assetPath: string, viewFile?: string): string {
  const trimmed = assetPath.trim()
  if (!trimmed || trimmed.includes('\0') || trimmed.includes('\\') || trimmed.startsWith('/') || trimmed.endsWith('/') || trimmed.includes('//')) {
    throw new Error('Asset path must be relative to the thread workspace')
  }
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed)) {
    throw new Error('Asset path must be relative to the thread workspace')
  }
  if (/[?#]/.test(trimmed)) {
    throw new Error('Asset path must not include query or fragment parameters')
  }
  const parts: string[] = []
  if (viewFile) {
    const viewParts = viewFile.trim().replace(/\\/g, '/').split('/').filter((segment) => segment && segment !== '.')
    if (viewParts.includes('..')) {
      throw new Error('View path must stay inside the thread workspace')
    }
    if (viewParts.length > 1) {
      parts.push(...viewParts.slice(0, -1))
    }
  }
  for (const segment of trimmed.split('/')) {
    if (!segment || segment === '.') continue
    if (segment === '..') {
      if (parts.length === 0) {
        throw new Error('Asset path must stay inside the thread workspace')
      }
      parts.pop()
      continue
    }
    parts.push(segment)
  }
  if (parts.length === 0) {
    throw new Error('Asset path is required')
  }
  return parts.join('/')
}

const REACT_CDN = 'https://esm.sh/react@19.2.0'
const REACT_DOM_CDN = 'https://esm.sh/react-dom@19.2.0'

const THEME_VARS = [
  '--color-bg',
  '--color-surface',
  '--color-surface-2',
  '--color-ink',
  '--color-ink-2',
  '--color-ink-3',
  '--color-primary',
  '--color-primary-strong',
  '--color-primary-soft',
  '--color-on-primary',
  '--color-border',
  '--color-danger',
  '--color-danger-soft',
  '--font-sans',
  '--font-mono',
  '--radius-control',
  '--radius-card',
]

const RAMPS = {
  purple: ['#EEEDFE', '#534AB7', '#26215C', '#3C3489', '#CECBF6'],
  teal: ['#E1F5EE', '#0F6E56', '#04342C', '#085041', '#9FE1CB'],
  coral: ['#FAECE7', '#993C1D', '#4A1B0C', '#712B13', '#F5C4B3'],
  pink: ['#FBEAF0', '#993556', '#4B1528', '#72243E', '#F4C0D1'],
  gray: ['#F1EFE8', '#5F5E5A', '#2C2C2A', '#444441', '#D3D1C7'],
  blue: ['#E6F1FB', '#185FA5', '#042C53', '#0C447C', '#B5D4F4'],
  green: ['#EAF3DE', '#3B6D11', '#173404', '#27500A', '#C0DD97'],
  amber: ['#FAEEDA', '#854F0B', '#412402', '#633806', '#FAC775'],
  red: ['#FCEBEB', '#A32D2D', '#501313', '#791F1F', '#F7C1C1'],
} as const

const bridgeScript = `
(() => {
  const post = (message) => parent.postMessage(message, '*');
  const measure = () => post({ type: 'jaz:artifact-height', height: Math.max(document.body?.scrollHeight || 0, document.documentElement.offsetHeight) });
  window.sendPrompt = (text, files) => post({ type: 'jaz:artifact-send-prompt', text: String(text || ''), files: Array.from(files || []) });
  window.openLink = (href) => post({ type: 'jaz:artifact-link', href: String(href || '') });
  window.openView = (name, title) => post({ type: 'jaz:artifact-open-view', name: String(name || ''), title: String(title || '') });
  window.addEventListener('error', (event) => post({ type: 'jaz:artifact-error', message: String(event.message || 'Artifact script error') }));
  window.addEventListener('unhandledrejection', (event) => post({ type: 'jaz:artifact-error', message: String(event.reason?.message || event.reason || 'Artifact promise rejection') }));
  document.addEventListener('click', (event) => {
    const link = event.target?.closest?.('a[href]');
    if (!link) return;
    event.preventDefault();
    window.openLink(link.getAttribute('href'));
  });
  new ResizeObserver(measure).observe(document.documentElement);
  window.addEventListener('load', measure);
  requestAnimationFrame(measure);
})();
`

const assetBridgeScript = `
(() => {
  let nextId = 1;
  const pending = new Map();
  window.addEventListener('message', (event) => {
    if (event.source !== parent) return;
    const message = event.data;
    if (message?.type !== 'jaz:artifact-resolve-asset-result' || typeof message.id !== 'string') return;
    const entry = pending.get(message.id);
    if (!entry) return;
    pending.delete(message.id);
    if (message.error) entry.reject(new Error(String(message.error)));
    else entry.resolve(message.asset);
  });
  const resolveAsset = (path) => {
    if (typeof path !== 'string' || !path.trim()) {
      return Promise.reject(new Error('Asset path is required'));
    }
    const id = 'asset-' + (nextId++);
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject });
      parent.postMessage({ type: 'jaz:artifact-resolve-asset', id, path }, '*');
    });
  };
  window.jaz = Object.assign(window.jaz || {}, { resolveAsset });
})();
`

const canvasBridgeScript = `
(() => {
  const fit = () => {
    const body = document.body
    if (!body || !innerWidth || !innerHeight) {
      return
    }
    body.style.transform = ''
    const scale = Math.min(1, innerWidth / body.scrollWidth, innerHeight / body.scrollHeight)
    body.style.transform = 'scale(' + scale + ')'
  }
  let pending = false
  const schedule = () => {
    if (pending) {
      return
    }
    pending = true
    requestAnimationFrame(() => {
      pending = false
      fit()
    })
  }
  document.addEventListener('DOMContentLoaded', () => {
    new ResizeObserver(schedule).observe(document.body)
    new MutationObserver((changes) => {
      if (changes.some((change) => change.target !== document.body)) {
        schedule()
      }
    }).observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true })
    fit()
  }, { once: true })
  window.addEventListener('resize', schedule)
  document.addEventListener('load', schedule, true)
  document.addEventListener('wheel', (event) => {
    event.preventDefault()
    const { clientX, clientY, deltaX, deltaY, deltaMode, ctrlKey, shiftKey } = event
    parent.postMessage({ type: 'jaz:canvas-wheel', wheel: { clientX, clientY, deltaX, deltaY, deltaMode, ctrlKey, shiftKey } }, '*')
  }, { passive: false })
})();
`

// What the thread's agent is doing, as a plugin's view hears it: asking means
// a question to the user is open in the chat.
export type AgentState = 'idle' | 'working' | 'asking'

function inlineJSON(value: unknown): string {
  return JSON.stringify(value ?? null).replace(/[<\u2028\u2029]/g, (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`)
}

// A view in a thread keeps one JSON value of its own, such as answers the
// person has not sent yet, which is there again when the view next opens.
function viewStateScript(state: unknown): string {
  return `
window.viewState = ${inlineJSON(state)}
window.setViewState = (value) => {
  window.viewState = value
  parent.postMessage({ type: 'jaz:artifact-view-state', state: value }, '*')
}
`
}

// A view draws `data` and hears of later changes, and of changes in what the
// thread's agent is doing. A card on the canvas also saves its data on its
// item, where the user's undo or the agent can change it.
function dataScript(data: unknown, agentState: AgentState = 'idle'): string {
  return `
(() => {
  const listeners = new Set()
  const watchers = new Set()
  window.data = ${inlineJSON(data)}
  window.agentState = '${agentState}'
  window.onAgentState = (listener) => {
    watchers.add(listener)
    return () => watchers.delete(listener)
  }
  window.saveData = (value) => {
    window.data = value
    parent.postMessage({ type: 'jaz:canvas-data', data: value }, '*')
  }
  window.onData = (listener) => {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }
  window.addEventListener('message', (event) => {
    if (event.source !== parent) {
      return
    }
    if (event.data?.type === 'jaz:canvas-data') {
      window.data = event.data.data
      listeners.forEach((listener) => listener(window.data))
    }
    if (event.data?.type === 'jaz:artifact-agent-state' && event.data.state !== window.agentState) {
      window.agentState = event.data.state
      watchers.forEach((listener) => listener(window.agentState))
    }
  })
})();
`
}

const canvasCSS = 'html,body{overflow:hidden!important}body{transform-origin:top left}*{scrollbar-width:none}'

// Tile-only extension: the widget author designs blind, so the host measures
// what actually rendered — dead space below the content, overflow past the
// tile, clipped elements, broken images — and reports it as jaz:artifact-layout.
// The loop sees the problems in its next-run prompt. Inline artifacts auto-size
// to content, so they never load this.
const widgetBridgeScript = `
(() => {
  const post = (data) => parent.postMessage(Object.assign({ type: 'jaz:artifact-layout' }, data), '*');
  const doc = document.documentElement;
  window.addEventListener('message', (event) => {
    const m = event && event.data;
    if (m && m.type === 'jaz:scale' && typeof m.scale === 'number') doc.style.zoom = String(m.scale);
  });
  function hideBroken() {
    let broken = 0;
    const imgs = document.images;
    for (let i = 0; i < imgs.length; i++) {
      const img = imgs[i];
      if (img.complete && img.naturalWidth === 0 && img.getAttribute('src')) { img.style.visibility = 'hidden'; broken++; }
    }
    return broken;
  }
  function measure() {
    const body = document.body;
    if (!body) return;
    const overflowPx = Math.max(0, doc.scrollHeight - doc.clientHeight);
    const nodes = body.querySelectorAll('*');
    let contentBottom = 0, clipped = 0;
    const count = Math.min(nodes.length, 600);
    for (let i = 0; i < count; i++) {
      const el = nodes[i];
      if (el.classList && el.classList.contains('sr-only')) continue;
      const style = getComputedStyle(el);
      if (el.scrollHeight > el.clientHeight + 2 && (style.overflowY === 'hidden' || style.overflowY === 'clip')) clipped++;
      if (el.childElementCount !== 0) continue;
      const rect = el.getBoundingClientRect();
      if (rect.width < 4 || rect.height < 4) continue;
      const visible = style.visibility !== 'hidden' && ((el.textContent && el.textContent.trim() !== '') || /^(IMG|SVG|CANVAS|VIDEO)$/.test(el.tagName) || (style.backgroundColor && !/rgba?\\(\\s*0\\s*,\\s*0\\s*,\\s*0\\s*,\\s*0\\s*\\)/.test(style.backgroundColor)) || parseFloat(style.borderTopWidth) > 0);
      if (visible && rect.bottom > contentBottom) contentBottom = rect.bottom;
    }
    const ch = doc.clientHeight;
    const deadPx = Math.max(0, (ch - 12) - contentBottom);
    const deadPct = overflowPx > 0 ? 0 : Math.round((100 * deadPx) / Math.max(1, ch));
    post({ dead_space_pct: deadPct, overflow_px: Math.round(overflowPx), clipped: clipped, img_errors: hideBroken() });
  }
  let timer = null;
  function schedule() { if (timer) clearTimeout(timer); timer = setTimeout(measure, 500); }
  document.addEventListener('error', (event) => {
    const el = event && event.target;
    if (el && el.tagName === 'IMG') { el.style.visibility = 'hidden'; schedule(); }
  }, true);
  if (typeof ResizeObserver === 'function') new ResizeObserver(schedule).observe(doc);
  window.addEventListener('load', () => setTimeout(measure, 800));
  setTimeout(measure, 800);
})();
`

function toolRecord(raw: unknown): Record<string, unknown> | undefined {
  if (typeof raw === 'string') {
    try { raw = JSON.parse(raw) } catch { return undefined }
  }
  return raw && typeof raw === 'object' ? raw as Record<string, unknown> : undefined
}

export function artifactToolError(raw: unknown): string {
  const result = toolRecord(raw)
  if (result?.status !== 'error' && result?.isError !== true) return ''
  return typeof result.error === 'string' ? result.error : 'Artifact failed.'
}

export function parseArtifactToolArgs(raw?: unknown): ArtifactInput | null {
  const input = toolRecord(raw)
  if (!input) return null
  const title = typeof input.title === 'string' ? input.title.trim() : ''
  const source = input.html ?? input.widget_code
  const code = typeof source === 'string' ? source : ''
  if (!title || !code.trim()) return null
  return {
    title,
    code,
    loadingMessages: loadingMessages(input.loading_messages),
  }
}

export function artifactInputFromEvent(artifact?: ArtifactEvent): ArtifactInput | null {
  if (!artifact?.title.trim() || !artifact.widget_code.trim()) return null
  return {
    title: artifact.title.trim(),
    code: artifact.widget_code,
    loadingMessages: loadingMessages(artifact.loading_messages),
  }
}

export function buildArtifactThemeCSS(darkOverride?: boolean): string {
  const root = document.documentElement
  const style = getComputedStyle(root)
  const values = Object.fromEntries(THEME_VARS.map((name) => [name, style.getPropertyValue(name).trim()]))
  return themeCSS(values, darkOverride ?? root.classList.contains('dark'))
}

// Inline artifacts, board widgets and canvas HTML share the theme and CSP.
// Each surface adds its own layout and interaction bridge.
export function buildArtifactDocument(
  input: ArtifactInput,
  theme: string,
  opts: {
    measureLayout?: boolean
    canvas?: boolean
    data?: unknown
    agentState?: AgentState
    viewState?: unknown
    pluginView?: boolean
    assetOrigin?: string
    resolveAsset?: boolean
  } = {},
): string {
  const csp = buildArtifactCSP(opts.pluginView, opts.assetOrigin)
  const data = opts.canvas || opts.data !== undefined ? dataScript(opts.data, opts.agentState) : ''
  const viewState = opts.viewState !== undefined ? viewStateScript(opts.viewState) : ''
  const assets = opts.resolveAsset ? assetBridgeScript : ''
  const bridge = bridgeScript + (opts.measureLayout ? widgetBridgeScript : '') + (opts.canvas ? canvasBridgeScript : '') + data + viewState + assets
  if (opts.canvas) {
    theme += canvasCSS
  }
  // A board tile fills a fixed cell, so the document height must resolve to the
  // viewport — then a fragment using height:100% fills the tile. Inline
  // artifacts grow to their content instead, so they must NOT get this.
  const css = opts.measureLayout ? `${artifactCSS(theme)} html,body{height:100%}` : artifactCSS(theme)
  if (isFullHTMLDocument(input.code)) return injectArtifactHost(input.code, theme, bridge, csp)
  return `<!doctype html><html><head><meta charset="utf-8">${contentSecurityMeta(csp)}<style>${css}</style><script>${bridge}</script></head><body>${input.code}</body></html>`
}

// A canvas component is CommonJS compiled from the agent's TSX. React and any
// other bare imports load from esm.sh, sharing one React instance.
export function buildComponentDocument(code: string, theme: string, data: unknown): string {
  const imports: Record<string, string> = {
    react: REACT_CDN,
    'react/jsx-runtime': `${REACT_CDN}/jsx-runtime`,
    'react-dom': REACT_DOM_CDN,
    'react-dom/client': `${REACT_DOM_CDN}/client`,
  }
  const packages = [...new Set([...code.matchAll(/require\("([^"]+)"\)/g)].map((match) => match[1]))]
  const main = `
const modules = {};
for (const name of ${JSON.stringify(packages)}) {
  modules[name] = await import(${JSON.stringify(Object.keys(imports))}.includes(name) ? name : 'https://esm.sh/' + name + '?external=react,react-dom');
}
const module = { exports: {} };
(function (require, module, exports) {
${code.replaceAll('</', '<\\/')}
})((name) => modules[name], module, module.exports);
const React = await import('react');
const { createRoot } = await import('react-dom/client');
createRoot(document.getElementById('root')).render(React.createElement(module.exports.default ?? module.exports));
`
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${ARTIFACT_CSP}"><style>${artifactCSS(theme)} html,body,#root{height:100%}${canvasCSS}</style><script>${bridgeScript}${canvasBridgeScript}${dataScript(data)}</script><script type="importmap">${JSON.stringify({ imports })}</script></head><body><div id="root"></div><script type="module">${main}</script></body></html>`
}

// A full document lays out its own page; a fragment is laid out by its host.
export function isFullHTMLDocument(code: string): boolean {
  const head = code.trimStart().slice(0, 64).toLowerCase()
  return head.startsWith('<!doctype') || head.startsWith('<html')
}

const HEAD_PREFIX = /^\s*(?:<!doctype\s[^>]*>\s*)?(?:<html(?:\s[^>]*)?>\s*)?<head(?:\s[^>]*)?>/i
const DOC_PREFIX = /^\s*(?:<!doctype\s[^>]*>\s*)?(?:<html(?:\s[^>]*)?>\s*)?/i

function injectArtifactHost(code: string, theme: string, bridge: string, csp: string): string {
  const support = `${contentSecurityMeta(csp)}<style>${theme}</style><script>${bridge}</script>`
  const head = HEAD_PREFIX.exec(code)
  if (head) {
    const end = head[0].length
    return code.slice(0, end) + support + code.slice(end)
  }
  const prefix = DOC_PREFIX.exec(code)
  const end = prefix ? prefix[0].length : 0
  return code.slice(0, end) + `<head>${support}</head>` + code.slice(end)
}

function contentSecurityMeta(csp: string): string {
  return `<meta http-equiv="Content-Security-Policy" content="${csp}">`
}

function loadingMessages(value: unknown): string[] {
  return Array.isArray(value)
    ? value.flatMap((item) => (typeof item === 'string' && item.trim() ? [item.trim()] : [])).slice(0, 4)
    : []
}

function themeCSS(values: Record<string, string>, dark: boolean): string {
  const vars = THEME_VARS.map((name) => `${name}:${values[name]};`).join('')
  const aliases = [
    '--color-background-primary:var(--color-bg);',
    '--color-background-secondary:var(--color-surface);',
    '--color-background-tertiary:var(--color-surface-2);',
    '--color-background-info:var(--color-primary-soft);',
    '--color-background-danger:var(--color-danger-soft);',
    '--color-text-primary:var(--color-ink);',
    '--color-text-secondary:var(--color-ink-2);',
    '--color-text-tertiary:var(--color-ink-3);',
    '--color-text-info:var(--color-primary-strong);',
    '--color-text-danger:var(--color-danger);',
    '--color-border-tertiary:var(--color-border);',
    '--color-border-secondary:color-mix(in oklab,var(--color-border),var(--color-ink) 18%);',
    '--color-border-primary:color-mix(in oklab,var(--color-border),var(--color-ink) 30%);',
    '--color-border-info:var(--color-primary);',
    '--color-border-danger:var(--color-danger);',
    '--font-serif:Georgia,serif;',
    '--border-radius-md:var(--radius-control);',
    '--border-radius-lg:var(--radius-card);',
  ].join('')
  const rampVars = Object.entries(RAMPS)
    .map(([name, ramp]) => {
      const [fill, stroke, title, darkFill, darkStroke] = ramp
      return dark
        ? `--artifact-${name}-fill:${darkFill};--artifact-${name}-stroke:${darkStroke};--artifact-${name}-text:${fill};`
        : `--artifact-${name}-fill:${fill};--artifact-${name}-stroke:${stroke};--artifact-${name}-text:${title};`
    })
    .join('')
  // color-scheme themes native controls/scrollbars and is the OS-independent
  // dark signal artifact JS reads (Jaz's dark mode is a class, not a media
  // query, so prefers-color-scheme inside the iframe is unreliable).
  const scheme = `color-scheme:${dark ? 'dark' : 'light'};`
  return `:root{${scheme}${vars}${aliases}${rampVars}}`
}

function artifactCSS(theme: string): string {
  const rampCSS = Object.keys(RAMPS)
    .map(
      (name) => `.c-${name}>rect,.c-${name}>circle,.c-${name}>ellipse,.c-${name}>polygon{fill:var(--artifact-${name}-fill);stroke:var(--artifact-${name}-stroke)}.c-${name}>text,.c-${name} .t,.c-${name} .ts,.c-${name} .th{fill:var(--artifact-${name}-text)}`,
    )
    .join('')
  return `${theme}
*{box-sizing:border-box}html{background:transparent;color:var(--color-ink);font-family:var(--font-sans);font-size:16px}body{margin:0;background:transparent;color:var(--color-ink);font-family:var(--font-sans);line-height:1.55;-webkit-font-smoothing:antialiased}button,input,select,textarea{font:inherit}button{min-height:40px;border-radius:var(--border-radius-md);border:.5px solid var(--color-border-secondary);background:transparent;color:var(--color-ink);padding:0 14px;cursor:pointer}button:hover{background:var(--color-surface-2)}a{color:var(--color-primary)}.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}svg{display:block;max-width:100%;height:auto}.t{font:400 14px var(--font-sans);fill:var(--color-ink)}.ts{font:400 12px var(--font-sans);fill:var(--color-ink-2)}.th{font:500 14px var(--font-sans);fill:var(--color-ink)}.box{fill:var(--color-surface);stroke:var(--color-border)}.arr{fill:none;stroke:var(--color-ink-3);stroke-width:1.5}.leader{fill:none;stroke:var(--color-border);stroke-width:.5;stroke-dasharray:3 3}.node{cursor:pointer}.node:hover{opacity:.86}${rampCSS}`
}
