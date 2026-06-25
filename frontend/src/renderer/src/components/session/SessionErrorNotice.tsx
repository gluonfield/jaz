import { Check, Copy } from 'lucide-react'
import { useCopyAction } from '@/lib/useCopyAction'

export function SessionErrorNotice({
  message,
  context,
  className = '',
}: {
  message: string
  context?: string
  className?: string
}) {
  const { copied, copy } = useCopyAction(message)

  return (
    <div role="alert" className={`min-w-0 max-w-[var(--prose-max)] rounded-card bg-surface px-3.5 py-3 ring-1 ring-danger/25 ${className}`}>
      <div className="flex items-center gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className="size-1.5 shrink-0 rounded-full bg-danger" aria-hidden />
          <span className="shrink-0 text-[11px] font-semibold tracking-[0.08em] uppercase text-danger">Error</span>
          {context ? <span className="truncate font-mono text-[11px] text-ink-3">{context}</span> : null}
        </div>
        <button
          type="button"
          aria-label="Copy error"
          title={copied ? 'Copied' : 'Copy error'}
          onClick={copy}
          className="relative -my-1.5 -mr-1.5 grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-[background-color,color,transform] duration-150 before:absolute before:-inset-1 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
        </button>
      </div>
      <p className="mt-1.5 whitespace-pre-wrap [overflow-wrap:break-word] text-[13px] leading-[1.55] text-ink select-text">{message}</p>
    </div>
  )
}
