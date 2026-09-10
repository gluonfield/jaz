import { useMutation } from '@tanstack/react-query'
import { ChevronDown, LoaderCircle, Square } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { useToast } from '@/components/ui/toast'
import { setAgentSessionConfig, stopAgentTask } from '@/lib/api/sessions'
import type { AgentSessionState, AgentTask } from '@/lib/api/types'

export function AgentSessionControls({
  sessionId,
  state,
  tasks,
  running,
  onCommand,
}: {
  sessionId: string
  state?: AgentSessionState
  tasks: AgentTask[]
  running: boolean
  onCommand: (text: string) => void
}) {
  const toast = useToast()
  const [command, setCommand] = useState('')
  const [args, setArgs] = useState('')
  const config = useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) => setAgentSessionConfig(sessionId, id, value),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  const commands = state?.commands ?? []
  const selectedCommand = commands.find((item) => item.name === command)
  const activeTasks = tasks.filter((task) => !task.connection_lost && (task.state === 'running' || task.state === 'paused'))
  const finishedTasks = tasks.filter((task) => task.connection_lost || (task.state !== 'running' && task.state !== 'paused'))
  if (!state && tasks.length === 0) return null

  return (
    <div className="mb-2 text-[12px] text-ink-2">
      {state?.notices?.map((notice) => <p key={notice} className="mb-2 px-3 text-ink-3">{notice}</p>)}
      {activeTasks.map((task) => <BackgroundTask key={task.id} sessionId={sessionId} task={task} />)}
      <details className="group rounded-xl bg-surface/70 px-3">
        <summary className="flex min-h-10 cursor-pointer list-none items-center gap-2 [&::-webkit-details-marker]:hidden">
          <span>Agent settings</span>
          {state?.auth ? <span className="min-w-0 flex-1 truncate text-ink-3">{state.auth.label}</span> : <span className="flex-1" />}
          <ChevronDown size={13} className="shrink-0 group-open:rotate-180" />
        </summary>
        <div className="flex max-h-[45vh] flex-col gap-3 overflow-y-auto pb-3">
          <div className="break-words">
            <p className="text-ink-3">Account reported by the agent</p>
            <p>{state?.auth?.label || 'Not reported'}</p>
            {state?.auth?.detail || state?.auth?.account?.email ? <p>{state.auth.detail || state.auth.account?.email}</p> : null}
            {state?.auth?.account?.organization ? <p>{state.auth.account.organization}</p> : null}
            {state?.auth?.account?.plan ? <p className="text-ink-3">Plan: {state.auth.account.plan}</p> : null}
          </div>
          {(state?.config_options ?? []).map((option) => {
            const values = option.options.map((value) => ({
              value: value.value,
              label: `${value.group ? `${value.group} · ` : ''}${value.name}${value.value === option.recommended_value ? ' (recommended)' : ''}`,
              description: value.description,
            }))
            if (!values.some((value) => value.value === option.current_value)) {
              values.unshift({ value: option.current_value, label: option.current_value || 'Not reported', description: undefined })
            }
            return (
              <div key={option.id} className="flex flex-wrap items-center justify-between gap-2" title={option.description}>
                <span>{option.name}</span>
                <Select
                  aria-label={option.name}
                  className="min-h-10 max-w-full sm:max-w-[70%]"
                  value={option.current_value}
                  options={values}
                  disabled={running || config.isPending || option.options.length === 0}
                  onChange={(value) => config.mutate({ id: option.id, value })}
                />
              </div>
            )
          })}
          {commands.length > 0 ? (
            <form className="flex flex-col gap-2" onSubmit={(event) => {
              event.preventDefault()
              if (!selectedCommand || running) return
              onCommand(`/${selectedCommand.name}${args ? ` ${args}` : ''}`)
              setArgs('')
            }}>
              <div className="flex items-center gap-2">
                <Select aria-label="Agent command" className="min-h-10 min-w-0 max-w-full" value={command} options={[
                  { value: '', label: 'Commands' },
                  ...commands.map((item) => ({ value: item.name, label: `/${item.name}`, description: item.description })),
                ]} onChange={(value) => {
                  setCommand(value)
                  setArgs('')
                }} disabled={running} />
                <Button type="submit" size="sm" className="min-h-10 shrink-0" disabled={!selectedCommand || running}>Run</Button>
              </div>
              {selectedCommand?.input_hint ? <Input aria-label="Command arguments" placeholder={selectedCommand.input_hint} value={args} onChange={(event) => setArgs(event.target.value)} disabled={running} /> : null}
            </form>
          ) : null}
          {finishedTasks.length > 0 ? (
            <details>
              <summary className="min-h-10 cursor-pointer py-2 text-ink-3">Previous background tasks ({finishedTasks.length})</summary>
              {finishedTasks.map((task) => <BackgroundTask key={task.id} sessionId={sessionId} task={task} />)}
            </details>
          ) : null}
        </div>
      </details>
    </div>
  )
}

function BackgroundTask({ sessionId, task }: { sessionId: string; task: AgentTask }) {
  const toast = useToast()
  const stop = useMutation({
    mutationFn: () => stopAgentTask(sessionId, task.id),
    onError: (error: Error) => toast(error.message, 'danger'),
  })
  return (
    <div className="flex min-h-10 items-center gap-2 px-2 py-1.5">
      {task.state === 'running' && !task.connection_lost ? <LoaderCircle size={13} className="shrink-0 animate-spin" /> : null}
      <div className="min-w-0 flex-1">
        <p className="truncate" title={task.name || task.description || task.id}>{task.name || task.description || task.id} <span className="text-ink-3">· {task.connection_lost ? 'Connection lost' : task.state || 'Status unknown'}</span></p>
        {task.summary ? <p className="truncate text-ink-3" title={task.summary}>{task.summary}</p> : null}
      </div>
      {task.can_stop && !task.connection_lost ? <Button size="sm" className="h-10 w-10 p-0" aria-label={`Stop ${task.name || task.id}`} disabled={stop.isPending} onClick={() => stop.mutate()}><Square size={12} /></Button> : null}
    </div>
  )
}
