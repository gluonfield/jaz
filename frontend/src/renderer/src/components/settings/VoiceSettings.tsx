import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Select } from '@/components/ui/Select'
import { Skeleton } from '@/components/ui/Skeleton'
import { updateVoiceSettings, voiceSettingsQuery } from '@/lib/api/liveVoice'
import type { SettingsSection } from '@/components/settings/sections'

export function VoiceSettings({ onNavigate }: { onNavigate: (section: SettingsSection) => void }) {
  const query = useQuery(voiceSettingsQuery)
  const client = useQueryClient()
  const save = useMutation({
    mutationFn: updateVoiceSettings,
    onSuccess: (settings) => client.setQueryData(voiceSettingsQuery.queryKey, settings),
  })

  if (query.isPending) {
    return <Skeleton className="mt-4 h-48" />
  }
  if (query.isError) {
    return <EmptyState title="Couldn't load voice settings">{query.error.message}</EmptyState>
  }

  return (
    <section className="py-4">
      <h1 className="text-lg font-semibold text-ink">Voice</h1>
      <p className="mt-0.5 text-[13px] text-ink-2">
        Talk with Jaz while your thread’s selected agent works in the background.
      </p>
      <SettingsCard className="mt-4 divide-y divide-border px-4">
        <div className="flex min-h-16 items-center justify-between gap-4 py-3">
          <div>
            <p className="text-[13px] font-medium text-ink">Voice agent</p>
            <p className="mt-0.5 text-[12px] text-ink-2">Handles listening and speaking.</p>
          </div>
          <span className="rounded-full bg-surface-2 px-3 py-1.5 text-[13px] text-ink">OpenAI</span>
        </div>
        <div className="flex min-h-16 items-center justify-between gap-4 py-3">
          <div>
            <p className="text-[13px] font-medium text-ink">Provider</p>
            <p className="mt-0.5 text-[12px] text-ink-2">How you connect to OpenAI.</p>
          </div>
          <Select
            aria-label="Voice provider"
            value={query.data.provider}
            disabled={save.isPending}
            options={query.data.providers.filter((provider) => provider.available || provider.id === query.data.provider).map((provider) => ({
              value: provider.id,
              label: provider.label,
              description: provider.reason,
            }))}
            onChange={(id) => {
              const provider = query.data.providers.find((provider) => provider.id === id)
              if (provider) {
                save.mutate({ provider: provider.id, voice: '' })
              }
            }}
          />
        </div>
        <div className="flex min-h-16 items-center justify-between gap-4 py-3">
          <div>
            <p className="text-[13px] font-medium text-ink">Voice</p>
            <p className="mt-0.5 text-[12px] text-ink-2">Applies to your next voice conversation.</p>
          </div>
          <Select
            aria-label="Speaking voice"
            value={query.data.voice}
            options={query.data.voices.map((voice) => ({ value: voice, label: voice.charAt(0).toUpperCase() + voice.slice(1) }))}
            disabled={save.isPending}
            onChange={(voice) => save.mutate({ provider: query.data.provider, voice })}
          />
        </div>
        {save.isError ? <p role="alert" className="py-3 text-[12px] text-danger">{save.error.message}</p> : null}
      </SettingsCard>
      <p className="mt-4 text-[13px] text-ink-2">
        Your thread keeps its selected agent and model, including Claude. Tool approvals appear in the chat. Provider changes apply to your next voice conversation.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" onClick={() => onNavigate('agents')}>
          Manage OAuth sign-in
        </Button>
        <Button variant="secondary" size="sm" onClick={() => onNavigate('providers')}>
          Manage API key
        </Button>
      </div>
    </section>
  )
}
