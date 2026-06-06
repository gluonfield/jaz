function prettyArgs(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

export function ToolCallCard({
  name,
  args,
  result,
}: {
  name: string
  args?: string
  result?: string
}) {
  return (
    <div className="overflow-hidden rounded-card border border-border">
      <p className="border-b border-border bg-surface px-3 py-1.5 font-mono text-[12px] font-medium text-ink-2">
        {name}
      </p>
      {args ? (
        <pre className="max-h-44 overflow-auto px-3 py-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-2">
          {prettyArgs(args)}
        </pre>
      ) : null}
      {result ? (
        <pre className="max-h-44 overflow-auto border-t border-border px-3 py-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-2">
          {result}
        </pre>
      ) : null}
    </div>
  )
}
