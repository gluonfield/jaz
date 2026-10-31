import { createFileRoute, useLocation } from '@tanstack/react-router'
import { ThreadView } from '@/components/session/ThreadView'

type SessionSearch = {
  message?: number
  page?: string
  item?: string
}

export const Route = createFileRoute('/sessions/$sessionId')({
  validateSearch: (search): SessionSearch => {
    const raw = search.message
    const message = typeof raw === 'number' ? raw : typeof raw === 'string' ? Number(raw) : 0
    const page = typeof search.page === 'string' ? search.page.trim() : undefined
    const item = typeof search.item === 'string' ? search.item.trim() : undefined
    return { ...(Number.isSafeInteger(message) && message > 0 ? { message } : {}), ...(page ? { page } : {}), ...(item ? { item } : {}) }
  },
  component: SessionRoute,
})

function SessionRoute() {
  const { sessionId } = Route.useParams()
  const { message, page, item } = Route.useSearch()
  const navigationKey = useLocation({ select: (location) => location.state.__TSR_key })
  const initialPrompt = useLocation({ select: (location) => location.state.initialSessionPrompt })
  return (
    <ThreadView
      key={sessionId}
      sessionId={sessionId}
      message={message}
      page={page}
      item={item}
      navigationKey={navigationKey}
      initialPrompt={initialPrompt?.sessionId === sessionId ? initialPrompt : undefined}
    />
  )
}
