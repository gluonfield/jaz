import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { useToast } from '@/components/ui/toast'
import {
  agentSettingsQuery,
  cloneAgentSettings,
  inputFromSettings,
  updateAgentSettings,
} from '@/lib/api/settings'
import type { AgentSettings as AgentSettingsData } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

function settingsKey(settings: AgentSettingsData | null): string {
  return settings ? JSON.stringify(inputFromSettings(settings)) : ''
}

// Shared shell for the two agent-settings screens: load the settings, hold an
// editable draft plus write-only provider keys, and expose a save that re-seeds
// the draft and refreshes dependent queries. `label` only varies the toast copy.
export function useAgentSettingsDraft(label: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const settings = useQuery(agentSettingsQuery)
  const [draft, setDraft] = useState<AgentSettingsData | null>(null)
  // The backend never returns provider key values, so they live outside the draft.
  const [providerKeys, setProviderKeys] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings.data) setDraft(cloneAgentSettings(settings.data))
  }, [settings.data])

  const save = useMutation({
    mutationFn: (input: AgentSettingsData) => updateAgentSettings(input, providerKeys),
    onSuccess: (saved) => {
      setDraft(cloneAgentSettings(saved))
      setProviderKeys({})
      toast(`Saved ${label}`)
    },
    onError: (error: Error) => toast(`Couldn't save ${label}: ${error.message}`, 'danger'),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const dirty = useMemo(
    () => settingsKey(draft) !== settingsKey(settings.data ?? null),
    [draft, settings.data],
  )
  const providerKeyDirty = Object.values(providerKeys).some((value) => value.trim().length > 0)

  return { settings, draft, setDraft, providerKeys, setProviderKeys, save, dirty, providerKeyDirty }
}

// The chrome both agent-settings screens share: heading, Save button, body slot.
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
  canSave: boolean
  saving: boolean
  onSave: () => void
  children: ReactNode
}) {
  return (
    <section className="py-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-ink">{title}</p>
          <p className="mt-0.5 text-pretty text-[13px] text-ink-2">{description}</p>
        </div>
        <Button variant="primary" size="md" disabled={!canSave} onClick={onSave}>
          <Save size={14} />
          {saving ? 'Saving...' : 'Save changes'}
        </Button>
      </div>
      <div className="mt-4">{children}</div>
    </section>
  )
}
