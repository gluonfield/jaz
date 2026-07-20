import type { ACPToolCall } from '@/lib/api/types'

export function toolNameKey(name?: string): string {
  return (name ?? '').toLowerCase().replace(/[\s_-]/g, '')
}

const toolPresentations: Record<string, { category?: string; label: string }> = {
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

const variantCategories: Record<string, string> = {
  bash: 'command',
  listdir: 'read',
  readfile: 'read',
  webfetch: 'web_fetch',
  websearch: 'web_search',
  xsearch: 'web_search',
}

const kindCategories: Record<string, string> = {
  delete: 'edit',
  edit: 'edit',
  execute: 'command',
  fetch: 'web_fetch',
  move: 'edit',
  read: 'read',
  search: 'search',
}

function toolVariant(call: ACPToolCall): string {
  const input = call.raw_input
  if (!input || typeof input !== 'object' || !('variant' in input)) return ''
  const variant = (input as Record<string, unknown>).variant
  return typeof variant === 'string' ? toolNameKey(variant) : ''
}

export function toolNameLabel(name?: string): string {
  return toolPresentations[toolNameKey(name)]?.label ?? name ?? ''
}

export function toolCallCategory(call: ACPToolCall): string {
  const category =
    toolPresentations[toolNameKey(call.tool_name)]?.category ||
    variantCategories[toolVariant(call)] ||
    kindCategories[(call.kind ?? '').toLowerCase()]
  if (category) return category
  const title = call.title ?? call.id
  if (/^edit\s/i.test(title)) return 'edit'
  if (/^read\s/i.test(title)) return 'read'
  if (/^search\s/i.test(title)) return 'search'
  if (/^view image\s/i.test(title)) return 'image'
  if (/^(command\s+-v|npx\s|npm\s|bun\s|go\s|git\s|python3?\s|tidy\s|wc\s|rg\s)/i.test(title)) return 'command'
  return 'tool'
}
