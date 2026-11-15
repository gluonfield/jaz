import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback, useMemo } from 'react'
import { ArtifactFrame } from '@/components/artifacts/ArtifactFrame'
import { EmptyState } from '@/components/ui/EmptyState'
import { pluginPageQuery, pluginsQuery } from '@/lib/api/plugins'
import { useTitlebarSlot } from '@/lib/titlebar'

export const Route = createFileRoute('/plugins/$plugin')({ component: PluginPage })

function PluginPage() {
  const { plugin } = Route.useParams()
  const page = useQuery(pluginPageQuery(plugin))
  const plugins = useQuery(pluginsQuery).data
  const title = plugins?.find((installed) => installed.name === plugin)?.page?.title ?? plugin
  const heading = useMemo(() => <span className="truncate text-[13px] font-medium text-ink-2">{title}</span>, [title])
  useTitlebarSlot(heading)
  const navigate = useNavigate()
  const input = useMemo(() => ({ title, code: page.data?.view ?? '', loadingMessages: [] }), [title, page.data?.view])
  const data = useMemo(() => ({ items: page.data?.items ?? [] }), [page.data?.items])
  const sendPrompt = useCallback((text: string) => {
    void navigate({ to: '/new', state: { draftPrompt: text } })
  }, [navigate])
  const openLink = useCallback((href: string) => {
    const session = /^\/sessions\/([^/?#]+)$/.exec(href)?.[1]
    if (!session) return false
    void navigate({ to: '/sessions/$sessionId', params: { sessionId: session } })
    return true
  }, [navigate])
  if (page.isPending) return <EmptyState title="Loading…" />
  if (page.isError) return <EmptyState title={`Couldn't open ${title}`}><p>{page.error.message}</p></EmptyState>
  return <ArtifactFrame key={plugin} input={input} data={data} onSendPrompt={sendPrompt} onOpenLink={openLink} />
}
