import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Server, Settings2, SquarePen } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { SETTINGS_SECTIONS, type SettingsSection } from '@/components/settings/sections'
import { searchThreads } from '@/lib/api/search'
import { keys } from '@/lib/query/keys'
import type { PaletteCommand, PaletteThread } from './commandPaletteTypes'

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
  return item.title.toLocaleLowerCase().includes(needle)
}

export function useCommandPaletteItems({
  open,
  query,
  onOpenChange,
  onOpenSettings,
  onOpenConnect,
}: {
  open: boolean
  query: string
  onOpenChange: (open: boolean) => void
  onOpenSettings: (section?: SettingsSection) => void
  onOpenConnect: () => void
}) {
  const navigate = useNavigate()
  const debouncedQuery = useDebouncedValue(query.trim(), 140)
  const searchEnabled = open && debouncedQuery.length >= 2

  const commands = useMemo<PaletteCommand[]>(
    () => [
      {
        id: 'new-thread',
        kind: 'command',
        title: 'New Thread',
        icon: SquarePen,
        shortcut: 'N',
        run: () => {
          onOpenChange(false)
          navigate({ to: '/new' })
        },
      },
      {
        id: 'connect-machine',
        kind: 'command',
        title: 'Connect to a machine',
        icon: Server,
        run: () => {
          onOpenChange(false)
          onOpenConnect()
        },
      },
      {
        id: 'settings',
        kind: 'command',
        title: 'Settings',
        icon: Settings2,
        run: () => {
          onOpenChange(false)
          onOpenSettings()
        },
      },
    ],
    [navigate, onOpenChange, onOpenSettings, onOpenConnect],
  )

  // Each settings section is its own action so the palette can jump straight to
  // Appearance, Memory, etc. They surface only once the user types (matched by
  // section name), keeping the resting palette uncluttered.
  const settingsCommands = useMemo<PaletteCommand[]>(
    () =>
      SETTINGS_SECTIONS.map((sectionItem) => ({
        id: `settings-${sectionItem.id}`,
        kind: 'command',
        title: sectionItem.label,
        icon: sectionItem.icon,
        run: () => {
          onOpenChange(false)
          onOpenSettings(sectionItem.id)
        },
      })),
    [onOpenChange, onOpenSettings],
  )

  // Archived chats stay searchable; the backend ranks them below active ones
  // and each result carries an `archived` flag for the UI badge. The key must
  // carry the same flag so it never collides with a non-archived search.
  const threadSearch = useQuery({
    queryKey: keys.threadSearch(debouncedQuery, true),
    queryFn: ({ signal }) =>
      searchThreads({
        query: debouncedQuery,
        includeArchived: true,
        limit: 16,
        signal,
      }),
    enabled: searchEnabled,
    staleTime: 15_000,
  })

  // The two sections are kept as their own typed lists (rendering consumes them
  // directly) plus a flat `items` whose order — commands first, then threads —
  // is the index space for keyboard navigation.
  const { commandItems, threadItems, items } = useMemo(() => {
    const baseItems = commands.filter((item) => commandMatches(item, query))
    const sectionItems = query.trim()
      ? settingsCommands.filter((item) => commandMatches(item, query))
      : []
    const commandItems = [...baseItems, ...sectionItems]
    const threadItems: PaletteThread[] =
      searchEnabled && threadSearch.data
        ? threadSearch.data.map((result) => ({
            id: `thread-${result.thread_id}-${result.message_seq ?? 0}`,
            kind: 'thread',
            result,
          }))
        : []
    return { commandItems, threadItems, items: [...commandItems, ...threadItems] }
  }, [commands, settingsCommands, query, searchEnabled, threadSearch.data])

  return {
    debouncedQuery,
    items,
    commandItems,
    threadItems,
    searchEnabled,
    threadSearch,
  }
}
