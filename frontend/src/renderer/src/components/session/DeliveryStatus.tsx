import { CircleAlert, Clock } from 'lucide-react'
import { createContext, useContext } from 'react'
import type { MessageDelivery } from '@/lib/api/types'

export const MessageDeliveryActions = createContext<{
  retry: (id: string) => void
  discard: (id: string) => void
} | null>(null)

const ACTION = 'relative cursor-pointer rounded transition-colors duration-150 before:absolute before:-inset-2 before:content-[\'\'] active:scale-[0.96]'

// Sits under a message the server has not accepted. A send that lands at once
// never shows it; a refused one stays, with the way to send it again.
export function DeliveryStatus({ delivery }: { delivery: MessageDelivery }) {
  const actions = useContext(MessageDeliveryActions)
  if (delivery.state === 'sending') {
    return (
      <p role="status" className="mt-0.5 flex animate-[delivery-wait_0.2s_ease-out_0.4s_backwards] items-center gap-1 text-[12px] text-ink-3">
        <Clock size={11} aria-hidden />
        Sending…
      </p>
    )
  }
  return (
    <p role="alert" className="mt-0.5 flex items-center gap-3 text-[12px]">
      <span className="flex items-center gap-1 whitespace-nowrap text-danger" title={delivery.error}>
        <CircleAlert size={12} aria-hidden />
        Not sent
      </span>
      <button type="button" className={`${ACTION} font-medium text-ink hover:underline`} onClick={() => actions?.retry(delivery.id)}>
        Retry
      </button>
      <button type="button" className={`${ACTION} text-ink-3 hover:text-ink`} onClick={() => actions?.discard(delivery.id)}>
        Delete
      </button>
    </p>
  )
}
