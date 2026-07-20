import { memo } from 'react'
import { Collapse } from '@/components/ui/Collapse'

function hasValue(value: unknown): boolean {
  return value !== undefined && value !== null && value !== ''
}

function formatValue(value: unknown): string {
  if (typeof value !== 'string') return JSON.stringify(value, null, 2) ?? String(value)
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

export function hasToolRawDetails(input: unknown, output: unknown): boolean {
  return hasValue(input) || hasValue(output)
}

export const ToolRawDetails = memo(function ToolRawDetails({
  open,
  input,
  output,
}: {
  open: boolean
  input?: unknown
  output?: unknown
}) {
  const fields = [
    { label: 'Input', value: input },
    { label: 'Output', value: output },
  ].filter((field) => hasValue(field.value))

  return (
    <Collapse open={open}>
      <div className="pb-2 pl-8 pt-1">
        <div className="divide-y divide-border/70 overflow-hidden rounded-card bg-surface/65 ring-1 ring-border/60">
          {fields.map((field) => (
            <div key={field.label} className="min-w-0">
              <div className="px-3 pt-2 text-[10px] font-medium uppercase tracking-[0.08em] text-ink-3">
                {field.label}
              </div>
              <pre className="scrollbar-quiet max-h-60 overflow-auto px-3 pb-2.5 pt-1 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
                {formatValue(field.value)}
              </pre>
            </div>
          ))}
        </div>
      </div>
    </Collapse>
  )
})
