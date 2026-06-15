import type { ArtifactEvent } from './api/types'

export interface ArtifactInput {
  title: string
  code: string
  loadingMessages: string[]
}

const ARTIFACT_CSP =
  "default-src 'none'; script-src 'unsafe-inline' https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com; style-src 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; font-src data: https://fonts.gstatic.com https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; img-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://fonts.gstatic.com https://fonts.googleapis.com; connect-src https://cdnjs.cloudflare.com https://esm.sh https://cdn.jsdelivr.net https://unpkg.com; media-src data: blob: https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com; frame-src 'none'; base-uri 'none'; form-action 'none'"

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
  '--color-border',
  '--color-danger',
  '--color-danger-soft',
  '--font-sans',
  '--font-mono',
  '--radius-control',
  '--radius-card',
]

const FALLBACKS: Record<string, string> = {
  '--color-bg': 'oklch(0.963 0.007 262)',
  '--color-surface': 'oklch(0.94 0.009 262)',
  '--color-surface-2': 'oklch(0.906 0.011 262)',
  '--color-ink': 'oklch(0.27 0.01 262)',
  '--color-ink-2': 'oklch(0.5 0.01 262)',
  '--color-ink-3': 'oklch(0.57 0.01 262)',
  '--color-primary': 'oklch(0.5 0.13 262)',
  '--color-primary-strong': 'oklch(0.43 0.13 262)',
  '--color-primary-soft': 'oklch(0.92 0.035 262)',
  '--color-border': 'oklch(0.853 0.01 262)',
  '--color-danger': 'oklch(0.55 0.17 25)',
  '--color-danger-soft': 'oklch(0.94 0.035 25)',
  '--font-sans': '"Anthropic Sans", ui-sans-serif, system-ui, sans-serif',
  '--font-mono': '"JetBrains Mono", ui-monospace, SFMono-Regular, monospace',
  '--radius-control': '10px',
  '--radius-card': '12px',
}

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
  const measure = () => post({ type: 'jaz:artifact-height', height: Math.max(document.body?.scrollHeight || 0, document.documentElement.scrollHeight) });
  window.sendPrompt = (text) => post({ type: 'jaz:artifact-send-prompt', text: String(text || '') });
  window.openLink = (href) => post({ type: 'jaz:artifact-link', href: String(href || '') });
  window.addEventListener('error', (event) => post({ type: 'jaz:artifact-error', message: String(event.message || 'Artifact script error') }));
  window.addEventListener('unhandledrejection', (event) => post({ type: 'jaz:artifact-error', message: String(event.reason?.message || event.reason || 'Artifact promise rejection') }));
  document.addEventListener('click', (event) => {
    const link = event.target?.closest?.('a[href]');
    if (!link) return;
    event.preventDefault();
    window.openLink(link.href);
  });
  new ResizeObserver(measure).observe(document.documentElement);
  window.addEventListener('load', measure);
  requestAnimationFrame(measure);
})();
`

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

export function parseArtifactToolArgs(raw?: string): ArtifactInput | null {
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null
  const input = parsed as Record<string, unknown>
  const title = typeof input.title === 'string' ? input.title.trim() : ''
  const code = typeof input.widget_code === 'string' ? input.widget_code : ''
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
  if (typeof window === 'undefined') return themeCSS(FALLBACKS, darkOverride ?? false)
  const root = document.documentElement
  const style = getComputedStyle(root)
  const values = Object.fromEntries(
    THEME_VARS.map((name) => [name, style.getPropertyValue(name).trim() || FALLBACKS[name]]),
  )
  return themeCSS(values, darkOverride ?? root.classList.contains('dark'))
}

// Inline artifacts and board widgets render through this one builder, so both
// get the same design system, theme, color-scheme, CSP, and CDN allowlist. The
// only difference is the bridge: board tiles also report layout telemetry back
// to their loop, opted in via measureLayout.
export function buildArtifactDocument(
  input: ArtifactInput,
  theme: string,
  opts: { measureLayout?: boolean } = {},
): string {
  const bridge = opts.measureLayout ? `${bridgeScript}${widgetBridgeScript}` : bridgeScript
  if (isFullHTMLDocument(input.code)) return injectArtifactHost(input.code, theme, bridge)
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${ARTIFACT_CSP}"><style>${artifactCSS(theme)}</style><script>${bridge}</script></head><body>${input.code}</body></html>`
}

function isFullHTMLDocument(code: string): boolean {
  const head = code.trimStart().slice(0, 64).toLowerCase()
  return head.startsWith('<!doctype') || head.startsWith('<html')
}

function injectArtifactHost(code: string, theme: string, bridge: string): string {
  const support = `${contentSecurityMeta(code)}<style>${theme}</style><script>${bridge}</script>`
  const headOpen = code.search(/<head(?:\s[^>]*)?>/i)
  if (headOpen >= 0) {
    const end = code.indexOf('>', headOpen)
    if (end >= 0) return code.slice(0, end + 1) + support + code.slice(end + 1)
  }
  const headClose = code.search(/<\/head\s*>/i)
  if (headClose >= 0) return code.slice(0, headClose) + support + code.slice(headClose)
  const bodyOpen = code.search(/<body(?:\s[^>]*)?>/i)
  if (bodyOpen >= 0) {
    const end = code.indexOf('>', bodyOpen)
    if (end >= 0) return code.slice(0, end + 1) + support + code.slice(end + 1)
  }
  return support + code
}

function contentSecurityMeta(code: string): string {
  return /content-security-policy/i.test(code)
    ? ''
    : `<meta http-equiv="Content-Security-Policy" content="${ARTIFACT_CSP}">`
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
