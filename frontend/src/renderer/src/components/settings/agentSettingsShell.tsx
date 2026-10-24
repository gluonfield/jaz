import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect, useReducer } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import { agentSettingsQuery, updateAgentSettings } from '@/lib/api/settings'
import type { AgentSettings as AgentSettingsData } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { agentSettingsDraft, agentSettingsDraftDirty } from '@/lib/agentSettingsDraft'

export function useAgentSettingsDraft(label: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [state, dispatch] = useReducer(agentSettingsDraft, null)
  const draft = state ? { ...state.server, ...state.input } : null
  const setDraft = (settings: AgentSettingsData) => dispatch({ type: 'edit', settings })

  useEffect(() => {
    if (settings.data) dispatch({ type: 'refresh', settings: settings.data })
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input),
    onSuccess: (saved) => {
      dispatch({ type: 'reset', settings: saved })
      queryClient.setQueryData(keys.agentSettings, saved)
      toast(`Saved ${label}`)
    },
    onError: (error: Error) => toast(`Couldn't save ${label}: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
      queryClient.invalidateQueries({ queryKey: keys.providerStatuses })
    },
  })

  return { settings, draft, setDraft, save, dirty: agentSettingsDraftDirty(state) }
}

// The chrome both agent-settings screens share: heading, optional save action, body slot.
export function SettingsSection({
  title,
  description,
  canSave,
  saving,
  onSave,
  children,
}: {
  title: string
  description: string
  canSave?: boolean
  saving?: boolean
  onSave?: () => void
  children: ReactNode
}) {
  return (
    <section className="py-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">{title}</p>
          <p className="mt-0.5 text-pretty text-[13px] text-ink-2">{description}</p>
        </div>
        {onSave ? (
          <Button variant="primary" size="md" disabled={!canSave} onClick={onSave}>
            <Save size={14} />
            {saving ? 'Saving...' : 'Save changes'}
          </Button>
        ) : null}
      </div>
      <div className="mt-3">{children}</div>
    </section>
  )
}
