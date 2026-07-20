import { memo } from 'react'

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
  input,
  output,
}: {
  input?: unknown
  output?: unknown
}) {
  const fields = [
    { label: 'Input', value: input },
    { label: 'Output', value: output },
  ].filter((field) => hasValue(field.value))

  return (
    <div className="pb-1 pl-7 pt-0.5">
      <div className="divide-y divide-border/60 overflow-hidden rounded-control bg-surface/55 ring-1 ring-border/50">
        {fields.map((field) => (
          <div key={field.label} className="min-w-0">
            <div className="px-2.5 pt-1.5 text-[10px] font-medium uppercase tracking-[0.08em] text-ink-3">
              {field.label}
            </div>
            <pre className="scrollbar-quiet max-h-60 overflow-auto px-2.5 pb-2 pt-0.5 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-ink-2 select-text">
              {formatValue(field.value)}
            </pre>
          </div>
        ))}
      </div>
    </div>
  )
})
