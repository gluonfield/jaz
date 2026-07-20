import type { ReactNode } from 'react'
import type { ACPToolCall } from '@/lib/api/types'
import { ArtifactBlock } from './ArtifactBlock'
import { ToolDisclosure } from './ToolDisclosure'
import { isArtifactToolName } from './toolVisibility'

interface ToolCallItem {
  key: string
  name: string
  args?: string
  result?: string
  pending?: boolean
}

function inputValue(raw?: string): unknown {
  if (!raw) return undefined
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

function ToolCallRun({ calls }: { calls: ToolCallItem[] }) {
  const toolCalls: ACPToolCall[] = calls.map((call) => ({
    id: call.key,
    tool_name: call.name,
    status: call.pending ? 'running' : 'completed',
    raw_input: inputValue(call.args),
    raw_output: call.result,
  }))
  return <ToolDisclosure calls={toolCalls} active={calls.some((call) => call.pending)} />
}

export function ToolCalls({
  calls,
  onArtifactPrompt,
}: {
  calls: ToolCallItem[]
  onArtifactPrompt?: (text: string) => void
}) {
  const rows: ReactNode[] = []
  let run: ToolCallItem[] = []
  const flush = () => {
    if (!run.length) return
    rows.push(<ToolCallRun key={`run-${run[0].key}`} calls={run} />)
    run = []
  }

  for (const call of calls) {
    if (!isArtifactToolName(call.name)) {
      run.push(call)
      continue
    }
    flush()
    rows.push(
      <ArtifactBlock
        key={call.key}
        args={call.args}
        result={call.result}
        pending={call.pending}
        onSendPrompt={onArtifactPrompt}
      />,
    )
  }
  flush()

  return <>{rows}</>
}
