import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'
import { normalized } from './TranscriptUtils'

export type ToolCategory =
  | 'web_search'
  | 'web_fetch'
  | 'edit'
  | 'read'
  | 'search'
  | 'image'
  | 'command'
  | 'tool'

export interface WebResult {
  url: string
  title: string
}

export type ToolPreview =
  | { type: 'web_results'; total: number; items: WebResult[] }
  | { type: 'web_fetch'; item: WebResult }

export interface ToolPresentation {
  category: ToolCategory
  label: string
  meta: string
  output?: unknown
  command?: string
  description?: string
  preview?: ToolPreview
}

const toolNames: Record<string, { category?: ToolCategory; label: string }> = {
  agent: { label: 'Agent' },
  applypatch: { category: 'edit', label: 'Edit file' },
  bash: { category: 'command', label: 'Bash' },
  edit: { category: 'edit', label: 'Edit' },
  execcommand: { category: 'command', label: 'Command' },
  glob: { category: 'search', label: 'Glob' },
  grep: { category: 'search', label: 'Grep' },
  ls: { category: 'read', label: 'List files' },
  multiedit: { category: 'edit', label: 'Edit' },
  notebookedit: { category: 'edit', label: 'Edit notebook' },
  notebookread: { category: 'read', label: 'Read notebook' },
  read: { category: 'read', label: 'Read' },
  readmcpresource: { category: 'read', label: 'Read resource' },
  task: { label: 'Task' },
  toolsearch: { category: 'search', label: 'Find tool' },
  todowrite: { label: 'Update plan' },
  updateplan: { label: 'Update plan' },
  viewimage: { category: 'image', label: 'View image' },
  webfetch: { category: 'web_fetch', label: 'Web fetch' },
  websearch: { category: 'web_search', label: 'Web search' },
  write: { category: 'edit', label: 'Write' },
  writestdin: { category: 'command', label: 'Terminal input' },
}

const kindCategories: Record<string, ToolCategory> = {
  delete: 'edit',
  edit: 'edit',
  execute: 'command',
  fetch: 'web_fetch',
  move: 'edit',
  read: 'read',
  search: 'search',
}

const categoryOrder: ToolCategory[] = [
  'web_search',
  'web_fetch',
  'read',
  'search',
  'command',
  'edit',
  'image',
  'tool',
]

const categoryPhrases: Record<ToolCategory, (count: number) => string> = {
  web_search: (count) => (count === 1 ? 'searched the web' : `searched the web ${count}×`),
  web_fetch: (count) => `visited ${count} page${count === 1 ? '' : 's'}`,
  edit: (count) => `edited ${count} file${count === 1 ? '' : 's'}`,
  read: (count) => `explored ${count} file${count === 1 ? '' : 's'}`,
  search: (count) => `searched ${count} time${count === 1 ? '' : 's'}`,
  image: (count) => `viewed ${count} image${count === 1 ? '' : 's'}`,
  command: (count) => `ran ${count} command${count === 1 ? '' : 's'}`,
  tool: (count) => `used ${count} tool${count === 1 ? '' : 's'}`,
}

export function toolNameKey(name?: string): string {
  return (name ?? '').toLowerCase().replace(/[\s_-]/g, '')
}

export function toolNameLabel(name?: string): string {
  return toolNames[toolNameKey(name)]?.label ?? name ?? ''
}

export function toolCallCategory(call: ACPToolCall): ToolCategory {
  const category =
    toolNames[toolNameKey(call.tool_name)]?.category || kindCategories[(call.kind ?? '').toLowerCase()]
  if (category) return category
  const title = call.title ?? call.id
  if (/^edit\s/i.test(title)) return 'edit'
  if (/^read\s/i.test(title)) return 'read'
  if (/^search\s/i.test(title)) return 'search'
  if (/^view image\s/i.test(title)) return 'image'
  if (/^(command\s+-v|npx\s|npm\s|bun\s|go\s|git\s|python3?\s|tidy\s|wc\s|rg\s)/i.test(title)) return 'command'
  return 'tool'
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  if (typeof value === 'string') {
    try {
      value = JSON.parse(value)
    } catch {
      return undefined
    }
  }
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function readField(value: unknown, key: string): string {
  const field = objectValue(value)?.[key]
  return typeof field === 'string' ? field.trim() : ''
}

function cleanTitle(title?: string): string {
  return (title ?? '').trim().replace(/^"+|"+$/g, '').replace(/:\s*$/, '')
}

export function toolDomain(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

function parseResultText(text: string): WebResult | undefined {
  const trimmed = text.trim()
  const paren = trimmed.match(/^(.*?)\s*\((https?:\/\/\S+)\)\s*$/)
  if (paren) return { title: paren[1].trim() || toolDomain(paren[2]), url: paren[2] }
  const url = trimmed.match(/https?:\/\/[^\s)]+/)?.[0]
  if (!url) return undefined
  const title = trimmed.replace(url, '').replace(/[()]/g, '').trim()
  return { title: title || toolDomain(url), url }
}

function webResults(content?: ACPToolContent[]): WebResult[] {
  const results: WebResult[] = []
  const seen = new Set<string>()
  for (const block of content ?? []) {
    const result =
      block.type === 'link' && block.uri
        ? { url: block.uri, title: block.title || toolDomain(block.uri) }
        : block.type === 'text' && block.text
          ? parseResultText(block.text)
          : undefined
    if (!result || seen.has(result.url)) continue
    seen.add(result.url)
    results.push(result)
  }
  return results
}

function searchQuery(call: ACPToolCall): string {
  const title = cleanTitle(call.title)
  return (
    readField(call.raw_input, 'query') ||
    (/^(web|x) search$/i.test(title) ? '' : title.replace(/^(web|x) search:\s*/i, ''))
  )
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

function inputHint(call: ACPToolCall): string {
  for (const key of ['file_path', 'path', 'target_file', 'target_directory', 'pattern', 'query', 'url']) {
    const value = readField(call.raw_input, key)
    if (value) return value
  }
  return ''
}

function commandText(call: ACPToolCall): string {
  const input = objectValue(call.raw_input)
  for (const key of ['command', 'cmd', 'script']) {
    const value = input?.[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (Array.isArray(value) && value.length) return value.map(String).join(' ')
  }
  return ''
}

function callLabel(call: ACPToolCall, category: ToolCategory, command: string, description: string): string {
  if (category === 'web_search') return searchQuery(call) || 'Web search'
  if (category === 'web_fetch') {
    const url = fetchURL(call)
    return toolDomain(url) || url || call.title || 'Web fetch'
  }
  if (category === 'command') {
    return description || command || cleanTitle(call.title) || toolNameLabel(call.tool_name) || 'Command'
  }
  const title = cleanTitle(call.title)
  const hint = inputHint(call)
  if (title && hint && !title.includes(hint)) return `${title} · ${hint}`
  return title || hint || toolNameLabel(call.tool_name) || 'Tool'
}

function callMeta(call: ACPToolCall, totalResults: number): string {
  if (normalized(call.status) === 'failed') return 'failed'
  if (totalResults) return `${totalResults} result${totalResults === 1 ? '' : 's'}`
  const elapsed = call.runtime?.elapsed_time_seconds
  if (elapsed !== undefined) return `${elapsed < 10 ? elapsed.toFixed(1) : Math.round(elapsed)}s`
  const status = normalized(call.status)
  return status && !['completed', 'complete', 'done'].includes(status) ? call.status ?? '' : ''
}

export function toolCallPresentation(call: ACPToolCall): ToolPresentation {
  const category = toolCallCategory(call)
  const command = commandText(call)
  const description = readField(call.raw_input, 'description')
  const results = category === 'web_search' ? webResults(call.content) : []
  const fetch = category === 'web_fetch' ? fetchURL(call) : ''
  return {
    category,
    label: callLabel(call, category, command, description),
    meta: callMeta(call, results.length),
    output: rawOutput(call),
    command: command || undefined,
    description: description || undefined,
    preview: results.length
      ? { type: 'web_results', total: results.length, items: results.slice(0, 3) }
      : fetch && normalized(call.status) !== 'failed'
        ? { type: 'web_fetch', item: { url: fetch, title: toolDomain(fetch) || fetch } }
        : undefined,
  }
}

export function hasToolCallDetail(call: ACPToolCall): boolean {
  const presentation = toolCallPresentation(call)
  return Boolean(
    call.title ||
      call.tool_name ||
      call.status ||
      call.raw_input !== undefined ||
      presentation.output !== undefined,
  )
}

export function toolRunLabel(calls: ACPToolCall[]): string {
  const counts = new Map<ToolCategory, number>()
  let failed = 0
  for (const call of calls) {
    const category = toolCallCategory(call)
    counts.set(category, (counts.get(category) ?? 0) + 1)
    if (normalized(call.status) === 'failed') failed += 1
  }
  const label = categoryOrder
    .flatMap((category) => {
      const count = counts.get(category)
      return count ? [categoryPhrases[category](count)] : []
    })
    .join(', ')
  const sentence = label.slice(0, 1).toUpperCase() + label.slice(1)
  return failed ? `${sentence}, ${failed} failed` : sentence
}
