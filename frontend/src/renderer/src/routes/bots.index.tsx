import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Navigate } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useCreateBot } from '@/components/bots/useCreateBot'
import { Button } from '@/components/ui/Button'
import { botsQuery } from '@/lib/api/bots'
import { dragSections, lastBotId, pinOrder } from '@/lib/bots'

export const Route = createFileRoute('/bots/')({
  component: BotsIndex,
})

function BotsIndex() {
  const create = useCreateBot()
  const bots = useQuery(botsQuery)
  const last = lastBotId()
  if (bots.isPending) return null
  const list = bots.data ?? []
  const { tiles, rows } = dragSections(list, pinOrder(list))
  const selected = list.find((bot) => bot.id === last) ?? tiles[0]?.bot ?? rows[0]?.bot
  if (selected) return <Navigate to="/bots/$botId" params={{ botId: selected.id }} search={(prev) => prev} replace />
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-8 text-center">
      <p className="text-sm text-ink-2">Bots keep working on their own, on your schedule.</p>
      <Button variant="primary" size="lg" disabled={create.isPending} onClick={() => create.mutate()}>
        <Plus size={15} />
        New bot
      </Button>
    </div>
  )
}
