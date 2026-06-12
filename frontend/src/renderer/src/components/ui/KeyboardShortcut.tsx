export function KeyboardShortcut({
  value,
  className = '',
}: {
  value: string | number
  className?: string
}) {
  return (
    <kbd
      className={`inline-flex h-5 min-w-11 items-center justify-center gap-1 rounded border border-border bg-bg px-1.5 font-sans text-[10px] leading-none tabular-nums text-ink-3 ${className}`}
    >
      <span className="text-[11px] leading-none">⌘</span>
      <span>{value}</span>
    </kbd>
  )
}
