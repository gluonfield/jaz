import { memo } from 'react'
import type { ACPToolCall } from '@/lib/api/types'
import { ActivityBlock } from './ActivityBlock'
import type { ActivityEntry } from './timeline'

export const ToolDisclosure = memo(function ToolDisclosure({
  calls,
  active = false,
  reveal = false,
}: {
  calls: ACPToolCall[]
  active?: boolean
  reveal?: boolean
}) {
  const entries: ActivityEntry[] = calls.map((call, index) => ({
    kind: 'tool',
    call,
    key: `tool-${call.id}-${index}`,
  }))
  return <ActivityBlock entries={entries} active={active} revealTools={reveal ? new Set(entries.map((entry) => entry.key)) : undefined} />
})
