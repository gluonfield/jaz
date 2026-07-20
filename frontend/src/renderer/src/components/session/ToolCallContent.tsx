import {
  ChevronRight,
  CircleEllipsis,
  ExternalLink,
  FilePenLine,
  FileText,
  Globe,
  Image,
  Search,
  SquareTerminal,
  type LucideIcon,
} from 'lucide-react'
import { memo, useState } from 'react'
import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'
import { toolCallCategory, toolNameKey } from '@/components/session/toolCallCategory'
import { EditDiffBlock, hasInlineDiff } from './EditDiffBlock'
import { hasToolRawDetails, ToolRawDetails } from './ToolRawDetails'
import { normalized } from './TranscriptUtils'

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
  return normalized(call.status) === 'failed'
}

function parseResultText(text: string): WebResult | null {
  const trimmed = text.trim()
  const paren = trimmed.match(/^(.*?)\s*\((https?:\/\/\S+)\)\s*$/)
  if (paren) return { title: paren[1].trim() || domainOf(paren[2]), url: paren[2] }
  const url = trimmed.match(/https?:\/\/[^\s)]+/)
  if (!url) return null
  const title = trimmed.replace(url[0], '').replace(/[()]/g, '').trim()
  return { title: title || domainOf(url[0]), url: url[0] }
}

function parseWebResults(content?: ACPToolContent[]): WebResult[] {
  if (!content?.length) return []
  const results: WebResult[] = []
  for (const block of content) {
    if (block.type === 'link' && block.uri) {
      results.push({ url: block.uri, title: block.title || domainOf(block.uri) })
    } else if (block.type === 'text' && block.text) {
      const result = parseResultText(block.text)
      if (result) results.push(result)
    }
  }
  return results
}

function readField(value: unknown, key: string): string {
  if (!value || typeof value !== 'object' || !(key in value)) return ''
  const field = (value as Record<string, unknown>)[key]
  return typeof field === 'string' ? field.trim() : ''
}

function toolNameLabel(name?: string): string {
  const key = toolNameKey(name)
  if (!key) return ''
  const labels: Record<string, string> = {
    agent: 'Agent',
    bash: 'Bash',
    edit: 'Edit',
    glob: 'Glob',
    grep: 'Grep',
    ls: 'List files',
    multiedit: 'Edit',
    notebookedit: 'Edit notebook',
    notebookread: 'Read notebook',
    read: 'Read',
    task: 'Task',
    todowrite: 'Update plan',
    webfetch: 'Web fetch',
    websearch: 'Web search',
    write: 'Write',
  }
  return labels[key] ?? name ?? ''
}

function searchQuery(call: ACPToolCall): string {
  return readField(call.raw_input, 'query') || (call.title ?? '').trim().replace(/^"+|"+$/g, '')
}

function fetchURL(call: ACPToolCall): string {
  const inputURL = readField(call.raw_input, 'url')
  if (inputURL) return inputURL
  const titleURL = (call.title ?? '').match(/https?:\/\/\S+/)?.[0]
  if (titleURL) return titleURL
  for (const block of call.content ?? []) {
    if (block.type === 'link' && block.uri) return block.uri
    const textURL = block.text?.match(/https?:\/\/\S+/)?.[0]
    if (textURL) return textURL
  }
  return ''
}

function previewText(content?: ACPToolContent[]): string {
  const parts: string[] = []
  for (const block of content ?? []) {
    if (block.type === 'text' && block.text) parts.push(block.text)
    else if (block.type === 'link' && block.uri)
      parts.push(block.title ? `${block.title} — ${block.uri}` : block.uri)
    else if (block.type === 'diff')
      parts.push(`${block.path ? `# ${block.path}\n` : ''}${block.new_text ?? ''}`)
  }
  return parts.join('\n\n').trim()
}

function rawOutput(call: ACPToolCall): unknown {
  return call.raw_output ?? (previewText(call.content) || undefined)
}

export function hasToolCallDetail(call: ACPToolCall): boolean {
  return Boolean(
    call.title ||
      call.tool_name ||
      call.status ||
      call.raw_input !== undefined ||
      rawOutput(call) !== undefined,
  )
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
      className="size-3.5 shrink-0 rounded-sm outline outline-1 outline-black/10 dark:outline-white/10"
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
      className="group flex min-h-10 w-full items-center gap-2 rounded-lg px-2 text-left transition-[background-color,transform] duration-150 hover:bg-surface active:scale-[0.96] motion-reduce:transition-none"
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

function inputHint(call: ACPToolCall): string {
  for (const key of ['file_path', 'path', 'pattern', 'query', 'url']) {
    const value = readField(call.raw_input, key)
    if (value) return value
  }
  return ''
}

function commandText(call: ACPToolCall): string {
  for (const key of ['command', 'cmd', 'script']) {
    const value = readField(call.raw_input, key)
    if (value) return value
  }
  return ''
}

function callLabel(call: ACPToolCall, category: string): string {
  if (category === 'web_search') return searchQuery(call) || 'Web search'
  if (category === 'web_fetch') {
    const url = fetchURL(call)
    return domainOf(url) || url || call.title || 'Web fetch'
  }
  if (category === 'command') return commandText(call) || call.title || 'Command'
  const title = (call.title ?? '').trim()
  const hint = inputHint(call)
  if (title && hint && !title.includes(hint)) return `${title} · ${hint}`
  return title || hint || toolNameLabel(call.tool_name) || 'Tool'
}

function callMeta(call: ACPToolCall, category: string): string {
  if (isFailed(call)) return 'failed'
  if (category === 'web_search') {
    const count = parseWebResults(call.content).length
    if (count) return `${count} result${count === 1 ? '' : 's'}`
  }
  const elapsed = call.runtime?.elapsed_time_seconds
  if (elapsed !== undefined) return `${elapsed < 10 ? elapsed.toFixed(1) : Math.round(elapsed)}s`
  const status = normalized(call.status)
  return status && !['completed', 'complete', 'done'].includes(status) ? call.status ?? '' : ''
}

function categoryIcon(category: string): LucideIcon {
  const icons: Record<string, LucideIcon> = {
    command: SquareTerminal,
    edit: FilePenLine,
    image: Image,
    read: FileText,
    search: Search,
    web_fetch: Globe,
    web_search: Search,
  }
  return icons[category] ?? CircleEllipsis
}

function HumanToolPreview({ call, category }: { call: ACPToolCall; category: string }) {
  if (category === 'web_search') {
    const results = parseWebResults(call.content)
    if (!results.length) return null
    return (
      <div className="mb-2 ml-8 flex flex-col rounded-card bg-surface/45 p-1 ring-1 ring-border/50">
        {results.map((result, index) => (
          <ResultRow key={`${index}-${result.url}`} url={result.url} title={result.title} />
        ))}
      </div>
    )
  }
  if (category === 'web_fetch') {
    const url = fetchURL(call)
    return url && !isFailed(call) ? (
      <div className="mb-2 ml-8 rounded-card bg-surface/45 p-1 ring-1 ring-border/50">
        <ResultRow url={url} title={domainOf(url) || url} />
      </div>
    ) : null
  }
  if (category === 'edit' && hasInlineDiff(call)) {
    return (
      <div className="mb-2 ml-8">
        <EditDiffBlock call={call} />
      </div>
    )
  }
  return null
}

export const ToolCallDetail = memo(function ToolCallDetail({ call }: { call: ACPToolCall }) {
  const [open, setOpen] = useState(false)
  const category = toolCallCategory(call)
  const output = rawOutput(call)
  const expandable = hasToolRawDetails(call.raw_input, output)
  const Icon = categoryIcon(category)
  const meta = callMeta(call, category)
  return (
    <div className="relative min-w-0">
      <button
        type="button"
        disabled={!expandable}
        aria-expanded={expandable ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="group flex min-h-10 w-full min-w-0 items-center gap-2 text-left transition-[color,transform] duration-150 enabled:hover:text-ink enabled:active:scale-[0.96] disabled:cursor-default"
      >
        <span className="relative z-[1] flex size-6 shrink-0 items-center justify-center rounded-full bg-bg text-ink-3 ring-1 ring-border/70">
          <Icon size={12} aria-hidden />
        </span>
        <span className="min-w-0 flex-1 truncate text-[12px] text-ink-2">
          {callLabel(call, category)}
        </span>
        {meta ? (
          <span className={`shrink-0 text-[11px] tabular-nums ${isFailed(call) ? 'text-danger' : 'text-ink-3'}`}>
            {meta}
          </span>
        ) : null}
        {expandable ? (
          <ChevronRight
            size={12}
            className={`mr-1 shrink-0 text-ink-3 transition-transform duration-150 motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
            aria-hidden
          />
        ) : null}
      </button>
      <HumanToolPreview call={call} category={category} />
      <ToolRawDetails open={open && expandable} input={call.raw_input} output={output} />
    </div>
  )
})
