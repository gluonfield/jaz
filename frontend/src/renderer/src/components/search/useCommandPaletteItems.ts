import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Settings, SquarePen } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { searchThreads } from '@/lib/api/search'
import { keys } from '@/lib/query/keys'
import type { PaletteCommand, PaletteItem, RoleMode } from './commandPaletteTypes'
import { roleModeRoles } from './commandPaletteTypes'

function useDebouncedValue(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay)
    return () => window.clearTimeout(timer)
  }, [delay, value])
  return debounced
}

function commandMatches(item: PaletteCommand, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase()
  if (!needle) return true
  return `${item.title} ${item.detail}`.toLocaleLowerCase().includes(needle)
}

export function useCommandPaletteItems({
  open,
  query,
  roleMode,
  onOpenChange,
  onOpenSettings,
}: {
  open: boolean
  query: string
  roleMode: RoleMode
  onOpenChange: (open: boolean) => void
  onOpenSettings: () => void
}) {
  const navigate = useNavigate()
  const debouncedQuery = useDebouncedValue(query.trim(), 140)
  const searchEnabled = open && debouncedQuery.length >= 2
  const roles = roleModeRoles(roleMode)
  const roleKey = roles.join(',')

  const commands = useMemo<PaletteCommand[]>(
    () => [
      {
        id: 'new-thread',
        kind: 'command',
        title: 'New Thread',
        detail: 'Start a fresh session',
        Icon: SquarePen,
        shortcut: 'N',
        run: () => {
          onOpenChange(false)
          navigate({ to: '/new' })
        },
      },
      {
        id: 'settings',
        kind: 'command',
        title: 'Settings',
        detail: 'Open app settings',
        Icon: Settings,
        run: () => {
          onOpenChange(false)
          onOpenSettings()
        },
      },
    ],
    [navigate, onOpenChange, onOpenSettings],
  )

  const threadSearch = useQuery({
    queryKey: keys.threadSearch(debouncedQuery, roleKey),
    queryFn: ({ signal }) =>
      searchThreads({
        query: debouncedQuery,
        roles,
        limit: 16,
        signal,
      }),
    enabled: searchEnabled,
    staleTime: 15_000,
  })

  const items = useMemo<PaletteItem[]>(() => {
    const commandItems = commands.filter((item) => commandMatches(item, query))
    const threadItems: PaletteItem[] =
      searchEnabled && threadSearch.data
        ? threadSearch.data.map((result) => ({
            id: `thread-${result.thread_id}-${result.message_seq ?? 0}`,
            kind: 'thread',
            result,
          }))
        : []
    return [...commandItems, ...threadItems]
  }, [commands, query, searchEnabled, threadSearch.data])

  return {
    debouncedQuery,
    items,
    searchEnabled,
    threadSearch,
  }
}
