import { memo } from 'react'
import type { ACPToolCall } from '@/lib/api/types'
import { ActivityDisclosure } from './ActivityDisclosure'
import type { ActivityEntry } from './timeline'

export const ToolDisclosure = memo(function ToolDisclosure({
  calls,
  active = false,
}: {
  calls: ACPToolCall[]
  active?: boolean
}) {
  const entries: ActivityEntry[] = calls.map((call, index) => ({
    kind: 'tool',
    call,
    key: `tool-${call.id}-${index}`,
  }))
  return <ActivityDisclosure entries={entries} active={active} />
})
