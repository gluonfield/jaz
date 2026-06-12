export function KeyboardShortcut({
  value,
  className = '',
}: {
  value: string | number
  className?: string
}) {
  return (
    <kbd
      className={`inline-flex h-[18px] min-w-8 items-center justify-center gap-0.5 rounded-full border border-border bg-bg px-1 font-sans text-[10px] leading-none tabular-nums text-ink-3 ${className}`}
    >
      <span className="text-[11px] leading-none">⌘</span>
      <span>{value}</span>
    </kbd>
  )
}
