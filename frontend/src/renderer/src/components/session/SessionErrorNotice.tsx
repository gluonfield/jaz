import { Check, CircleAlert, Copy } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { writeClipboard } from '@/lib/clipboard'

export function SessionErrorNotice({ message, context }: { message: string; context?: string }) {
  const [copied, setCopied] = useState(false)
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const copy = useCallback(async () => {
    if (!(await writeClipboard(message))) return
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    setCopied(true)
    copiedTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [message])

  useEffect(() => () => {
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
  }, [])

  return (
    <div role="alert" className="flex max-w-[72ch] items-start gap-3 rounded-card bg-danger-soft px-3.5 py-3 text-sm text-danger ring-1 ring-danger/20">
      <CircleAlert size={16} className="mt-0.5 shrink-0" aria-hidden />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center justify-between gap-3">
          <p className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.08em] uppercase">
            Error
            {context ? <span className="ml-2 font-mono font-normal normal-case tracking-normal">{context}</span> : null}
          </p>
          <button
            type="button"
            aria-label="Copy error"
            title={copied ? 'Copied' : 'Copy error'}
            onClick={copy}
            className="-my-2 -mr-2 grid size-10 shrink-0 cursor-pointer place-items-center rounded-full transition-[background-color,color,transform] duration-150 hover:bg-surface/70 active:scale-[0.96]"
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
          </button>
        </div>
        <p className="mt-1 whitespace-pre-wrap break-words leading-[1.55] select-text">{message}</p>
      </div>
    </div>
  )
}
