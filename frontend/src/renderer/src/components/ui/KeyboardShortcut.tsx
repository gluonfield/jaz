export function KeyboardShortcut({
  value,
  modifier = '⌘',
  className = '',
}: {
  value: string | number
  modifier?: string
  className?: string
}) {
  return (
    <kbd
      className={`inline-flex h-[18px] min-w-8 items-center justify-center gap-0.5 rounded-full border border-border bg-bg px-1 font-sans text-[10px] leading-none tabular-nums text-ink-3 ${className}`}
    >
      {modifier ? <span className="text-[11px] leading-none">{modifier}</span> : null}
      <span>{value}</span>
    </kbd>
  )
}
