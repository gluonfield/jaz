import { LoaderCircle } from 'lucide-react'
import { memo } from 'react'
import type { ACPToolCall, ACPToolContent } from '@/lib/api/types'
import { toolCallCategory } from './ToolCallContent'
import { normalized } from './TranscriptUtils'

const RUNNING_STATUSES = new Set(['pending', 'in_progress', 'in-progress', 'running'])

export function hasInlineShellCommand(call: ACPToolCall): boolean {
  return toolCallCategory(call) === 'command'
}

// The executed command. Agents disagree on shape: Claude Code sends a `command`
// string, the native exec tool sends `cmd`, Codex sends an argv array. The
// disclosure title is the reliable cross-agent fallback.
function commandText(call: ACPToolCall): string {
  const input = call.raw_input
  if (input && typeof input === 'object') {
    const record = input as Record<string, unknown>
    for (const key of ['command', 'cmd', 'script']) {
      const value = record[key]
      if (typeof value === 'string' && value.trim()) return value.trim()
      if (Array.isArray(value) && value.length) return value.map(String).join(' ')
    }
  }
  return (call.title ?? '').trim()
}

function commandDescription(call: ACPToolCall): string {
  const input = call.raw_input
  if (input && typeof input === 'object') {
    const desc = (input as Record<string, unknown>).description
    if (typeof desc === 'string') return desc.trim()
  }
  return ''
}

// Some agents (e.g. Codex) wrap command output in a markdown code fence; we
// already render it monospace, so unwrap an enclosing ```lang … ``` block. The
// opening and closing fences are handled independently so streaming output that
// hasn't been closed yet still loses its leading fence.
function stripWrappingFence(text: string): string {
  const trimmed = text.trim()
  if (!/^```[^\n]*\n/.test(trimmed)) return text
  return trimmed.replace(/^```[^\n]*\n/, '').replace(/\n?```\s*$/, '')
}

// Combined stdout/stderr: the runtime delivers command output as text blocks.
function outputText(content?: ACPToolContent[]): string {
  if (!content?.length) return ''
  const joined = content
    .filter((block): block is ACPToolContent & { text: string } => block.type === 'text' && !!block.text)
    .map((block) => block.text)
    .join('\n')
    .replace(/\s+$/, '')
  return stripWrappingFence(joined)
}

// Terminal-style inline view for a shell command: the command up top with its
// exit status, the captured output below. Mirrors EditDiffBlock's bordered card.
export const ShellCommandBlock = memo(function ShellCommandBlock({
  call,
  active = false,
}: {
  call: ACPToolCall
  active?: boolean
}) {
  const command = commandText(call)
  const description = commandDescription(call)
  const output = outputText(call.content)
  const exitCode = call.runtime?.terminal_exit_code
  const failed = normalized(call.status) === 'failed' || (exitCode !== undefined && exitCode !== 0)
  const running = active && RUNNING_STATUSES.has(normalized(call.status))

  return (
    <div className="w-full overflow-hidden rounded-card border border-border">
      <div className="bg-surface px-2.5 py-1.5">
        <div className="flex items-start gap-2">
          <span className="mt-px shrink-0 font-mono text-[12px] leading-relaxed text-ink-3 select-none" aria-hidden>
            $
          </span>
          {/* One line: long commands scroll horizontally rather than wrapping;
              the full text is available on hover (desktop) and by swiping the
              row (touch). */}
          <pre
            title={command || undefined}
            className="min-w-0 flex-1 overflow-x-auto font-mono text-[12px] leading-relaxed whitespace-pre text-ink select-text"
          >
            {command || '(command)'}
          </pre>
          {running ? (
            <LoaderCircle className="mt-0.5 size-3 shrink-0 animate-spin text-running" aria-hidden />
          ) : exitCode !== undefined ? (
            <span
              className={`mt-px shrink-0 font-mono text-[11px] tabular-nums ${exitCode === 0 ? 'text-ink-3' : 'text-danger'}`}
            >
              exit {exitCode}
            </span>
          ) : failed ? (
            <span className="mt-px shrink-0 text-[11px] text-danger">failed</span>
          ) : null}
        </div>
        {description ? <p className="mt-1 pl-3.5 text-[11px] text-ink-3">{description}</p> : null}
      </div>
      {output ? (
        <pre className="max-h-72 overflow-auto border-t border-border px-2.5 py-2 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
          {output}
        </pre>
      ) : null}
    </div>
  )
})
