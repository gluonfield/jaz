import type { ACPToolCall } from '@/lib/api/types'

export function toolNameKey(name?: string): string {
  return (name ?? '').toLowerCase().replace(/[\s_-]/g, '')
}

export function toolCallCategory(call: ACPToolCall): string {
  const name = toolNameKey(call.tool_name)
  if (name === 'websearch') return 'web_search'
  if (name === 'webfetch' || (call.kind ?? '').toLowerCase() === 'fetch') return 'web_fetch'
  if (name === 'bash') return 'command'
  if (name === 'read' || name === 'notebookread' || name === 'ls') return 'read'
  if (name === 'grep' || name === 'glob') return 'search'
  if (name === 'edit' || name === 'multiedit' || name === 'write' || name === 'notebookedit') {
    return 'edit'
  }
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
