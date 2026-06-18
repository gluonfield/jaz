import { Check, Copy } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { writeClipboard } from '@/lib/clipboard'
import { MessageMarkdown } from './MessageMarkdown'

export function AssistantMarkdown({ text }: { text: string }) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <MessageMarkdown text={text} />
      <AssistantCopyButton text={text} />
    </div>
  )
}

function AssistantCopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reduceMotion = useReducedMotion()

  const copyMessage = useCallback(async () => {
    if (!(await writeClipboard(text))) return
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
    setCopied(true)
    copiedTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [text])

  useEffect(() => {
    setCopied(false)
  }, [text])

  useEffect(() => () => {
    if (copiedTimer.current) clearTimeout(copiedTimer.current)
  }, [])

  return (
    <button
      type="button"
      aria-label={copied ? 'Copied message as Markdown' : 'Copy message as Markdown'}
      title={copied ? 'Copied' : 'Copy message as Markdown'}
      onClick={() => void copyMessage()}
      className="group mt-0.5 inline-flex h-7 w-fit cursor-pointer items-center gap-1.5 rounded-full px-2 text-[12px] font-medium text-ink-3 transition-[background-color,color,transform] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96]"
    >
      <span className="grid size-3.5 shrink-0 place-items-center">
        <AnimatePresence initial={false} mode="popLayout">
          <motion.span
            key={copied ? 'copied' : 'copy'}
            initial={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
            animate={reduceMotion ? { opacity: 1 } : { opacity: 1, scale: 1, filter: 'blur(0px)' }}
            exit={reduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
            transition={reduceMotion ? { duration: 0.12 } : { type: 'spring', duration: 0.3, bounce: 0 }}
            className="grid size-3.5 place-items-center"
          >
            {copied ? (
              <Check size={14} className="text-primary" aria-hidden />
            ) : (
              <Copy size={14} className="text-ink-3 transition-colors group-hover:text-ink" aria-hidden />
            )}
          </motion.span>
        </AnimatePresence>
      </span>
      <span>{copied ? 'Copied' : 'Copy'}</span>
    </button>
  )
}
