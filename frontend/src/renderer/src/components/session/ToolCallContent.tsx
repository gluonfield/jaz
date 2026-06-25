import { ChevronRight, ExternalLink, Globe, Search } from 'lucide-react'
import { memo, useState } from 'react'
import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'

interface WebResult {
  url: string
  title: string
}

function domainOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

function openExternal(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer')
}

function isFailed(call: ACPToolCall): boolean {
  return (call.status ?? '').trim().toLowerCase() === 'failed'
}

function parseResultText(text: string): WebResult | null {
  const trimmed = text.trim()
  // Greedy \S+ anchored on the trailing ")" keeps inner-paren URLs intact
  // (e.g. wikipedia /Bar_(baz)) without swallowing spaces.
  const paren = trimmed.match(/^(.*?)\s*\((https?:\/\/\S+)\)\s*$/)
  if (paren) return { title: paren[1].trim() || domainOf(paren[2]), url: paren[2] }
  const url = trimmed.match(/https?:\/\/[^\s)]+/)
  if (url) {
    const title = trimmed.replace(url[0], '').replace(/[()]/g, '').trim()
    return { title: title || domainOf(url[0]), url: url[0] }
  }
  return null
}

function parseWebResults(content?: ACPToolContent[]): WebResult[] {
  if (!content?.length) return []
  const out: WebResult[] = []
  for (const block of content) {
    if (block.type === 'link' && block.uri) {
      out.push({ url: block.uri, title: block.title || domainOf(block.uri) })
    } else if (block.type === 'text' && block.text) {
      const result = parseResultText(block.text)
      if (result) out.push(result)
    }
  }
  return out
}

function readField(value: unknown, key: string): string {
  if (value && typeof value === 'object' && key in value) {
    const field = (value as Record<string, unknown>)[key]
    if (typeof field === 'string') return field.trim()
  }
  return ''
}

function searchQuery(call: ACPToolCall): string {
  const fromInput = readField(call.raw_input, 'query')
  if (fromInput) return fromInput
  return (call.title ?? '').trim().replace(/^"+|"+$/g, '')
}

function fetchURL(call: ACPToolCall): string {
  const fromInput = readField(call.raw_input, 'url')
  if (fromInput) return fromInput
  const fromTitle = (call.title ?? '').match(/https?:\/\/\S+/)
  if (fromTitle) return fromTitle[0]
  for (const block of call.content ?? []) {
    if (block.type === 'link' && block.uri) return block.uri
    if (block.text) {
      const match = block.text.match(/https?:\/\/\S+/)
      if (match) return match[0]
    }
  }
  return ''
}

// webToolVariant resolves whether a call is a web search or a single page fetch.
// tool_name is the reliable signal (Claude sets WebSearch/WebFetch); other
// agents only give kind — and Claude even tags both web tools as "fetch" — so we
// fall back to result shape and the title verb.
function webToolVariant(call: ACPToolCall): 'search' | 'fetch' | null {
  const name = (call.tool_name ?? '').toLowerCase().replace(/[\s_-]/g, '')
  if (name === 'websearch') return 'search'
  if (name === 'webfetch') return 'fetch'
  const kind = (call.kind ?? '').toLowerCase()
  if (kind !== 'fetch' && kind !== 'search') return null
  // `kind` alone is ambiguous: a filesystem search or directory listing also
  // reports kind 'search'. Only treat it as a web tool when there's an actual
  // URL — in the results or the title — otherwise let it fall through to the
  // local search category instead of "Searched the web".
  const results = parseWebResults(call.content)
  const urlInTitle = /https?:\/\//i.test(call.title ?? '')
  if (!results.length && !urlInTitle) return null
  if (results.length > 1) return 'search'
  if (/search/i.test(call.title ?? '')) return 'search'
  return kind === 'search' ? 'search' : 'fetch'
}

export function toolCallCategory(call: ACPToolCall): string {
  const web = webToolVariant(call)
  if (web === 'search') return 'web_search'
  if (web === 'fetch') return 'web_fetch'
  const kind = (call.kind ?? '').toLowerCase()
  if (kind === 'edit' || kind === 'delete' || kind === 'move') return 'edit'
  if (kind === 'read') return 'read'
  if (kind === 'search') return 'search'
  if (kind === 'execute') return 'command'
  const title = call.title ?? call.id
  if (/^edit\s/i.test(title)) return 'edit'
  if (/^read\s/i.test(title)) return 'read'
  if (/^search\s/i.test(title)) return 'search'
  if (/^view image\s/i.test(title)) return 'image'
  if (/^(command\s+-v|npx\s|npm\s|bun\s|go\s|git\s|python3?\s|tidy\s|wc\s|rg\s)/i.test(title)) return 'command'
  return 'tool'
}

function previewText(content?: ACPToolContent[]): string {
  if (!content?.length) return ''
  const parts: string[] = []
  for (const block of content) {
    if (block.type === 'text' && block.text) parts.push(block.text)
    else if (block.type === 'link' && block.uri)
      parts.push(block.title ? `${block.title} — ${block.uri}` : block.uri)
    else if (block.type === 'diff')
      parts.push(`${block.path ? `# ${block.path}\n` : ''}${block.new_text ?? ''}`)
  }
  return parts.join('\n\n').trim()
}

const Favicon = memo(function Favicon({ url }: { url: string }) {
  const [failed, setFailed] = useState(false)
  const domain = domainOf(url)
  if (failed || !domain) return <Globe size={14} className="size-3.5 shrink-0 text-ink-3" aria-hidden />
  return (
    <img
      src={`https://www.google.com/s2/favicons?domain=${encodeURIComponent(domain)}&sz=64`}
      alt=""
      width={14}
      height={14}
      loading="lazy"
      onError={() => setFailed(true)}
      className="size-3.5 shrink-0 rounded-sm"
    />
  )
})

const ResultRow = memo(function ResultRow({ url, title }: WebResult) {
  const domain = domainOf(url)
  return (
    <button
      type="button"
      onClick={() => openExternal(url)}
      title={url}
      className="group flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left transition-colors hover:bg-surface"
    >
      <Favicon url={url} />
      <span className="min-w-0 flex-1 truncate text-[12px] text-ink">{title || domain || url}</span>
      {domain ? <span className="shrink-0 text-[11px] text-ink-3">{domain}</span> : null}
      <ExternalLink
        size={11}
        className="shrink-0 text-ink-3 opacity-0 transition-opacity group-hover:opacity-100"
        aria-hidden
      />
    </button>
  )
})

function WebSearchDetail({ call }: { call: ACPToolCall }) {
  const query = searchQuery(call)
  const results = parseWebResults(call.content)
  return (
    <div className="flex w-full flex-col gap-1">
      <div className="flex items-center gap-1.5 text-[12px] text-ink-2">
        <Search size={12} className="shrink-0 text-ink-3" aria-hidden />
        <span className="min-w-0 truncate">{query || 'Web search'}</span>
        {results.length ? (
          <span className="shrink-0 text-ink-3">
            · {results.length} result{results.length === 1 ? '' : 's'}
          </span>
        ) : null}
        {isFailed(call) ? <span className="shrink-0 text-danger">· failed</span> : null}
      </div>
      {results.length ? (
        <div className="flex flex-col">
          {results.map((result, index) => (
            <ResultRow key={`${index}-${result.url}`} url={result.url} title={result.title} />
          ))}
        </div>
      ) : null}
    </div>
  )
}

function WebFetchDetail({ call }: { call: ACPToolCall }) {
  const url = fetchURL(call)
  if (!url || isFailed(call)) return <GenericToolRow call={call} />
  return <ResultRow url={url} title={domainOf(url) || url} />
}

function GenericToolRow({ call }: { call: ACPToolCall }) {
  const [open, setOpen] = useState(false)
  const detail = previewText(call.content)
  const hasDetail = Boolean(detail)
  return (
    <div className="flex w-full flex-col gap-1">
      <button
        type="button"
        disabled={!hasDetail}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex max-w-full items-center gap-1 text-left font-mono text-[11px] text-ink-2 transition-colors enabled:hover:text-ink disabled:cursor-default"
      >
        {hasDetail ? (
          <ChevronRight
            size={11}
            className={`shrink-0 transition-transform ${open ? 'rotate-90' : ''}`}
            aria-hidden
          />
        ) : null}
        <span className="truncate">{call.title || call.id}</span>
        {call.status ? <span className="text-ink-3"> · {call.status}</span> : null}
      </button>
      {open && hasDetail ? (
        <pre className="ml-3 max-h-44 overflow-auto rounded-card bg-surface px-2 py-1.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
          {detail}
        </pre>
      ) : null}
    </div>
  )
}

export const ToolCallDetail = memo(function ToolCallDetail({ call }: { call: ACPToolCall }) {
  const category = toolCallCategory(call)
  if (category === 'web_search') return <WebSearchDetail call={call} />
  if (category === 'web_fetch') return <WebFetchDetail call={call} />
  return <GenericToolRow call={call} />
})
