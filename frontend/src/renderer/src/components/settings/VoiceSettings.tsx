import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy } from 'lucide-react'
import { ASSISTANT_URLS, type AssistantWakeMode } from '@shared/assistant'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { Skeleton } from '@/components/ui/Skeleton'
import { useAssistantWakeMode } from '@/lib/assistant/settings'
import { updateVoiceSettings, voiceSettingsQuery } from '@/lib/api/liveVoice'
import { clientRuntime } from '@/lib/clientRuntime'

const WAKE_MODES: { value: AssistantWakeMode; label: string; description: string }[] = [
  {
    value: 'dictate',
    label: 'Quick dictate to primary bot',
    description: 'Opens a top-of-screen listening pill and sends after you pause, without switching apps.',
  },
  {
    value: 'voice',
    label: 'Open live voice with primary bot',
    description: 'Brings Jaz forward and starts a live voice conversation with your primary bot.',
  },
]

const AUTOMATION_LINKS = [
  { label: 'Default ("Hey Jaz")', url: ASSISTANT_URLS.default, note: 'Runs the selected behavior above' },
  { label: 'Mode A — Live voice', url: ASSISTANT_URLS.voice, note: 'Always opens primary bot live voice' },
  { label: 'Mode B — Quick dictate', url: ASSISTANT_URLS.dictate, note: 'Always opens top-of-screen listen-and-send' },
  { label: 'Mode B — Send dictated text', url: `${ASSISTANT_URLS.sendExample}Hello`, note: 'Sends URL-encoded text straight to primary bot' },
]

export function VoiceSettings() {
  const query = useQuery(voiceSettingsQuery)
  const client = useQueryClient()
  const [wakeMode, setWakeMode] = useAssistantWakeMode()
  const [copiedUrl, setCopiedUrl] = useState('')
  const save = useMutation({
    mutationFn: updateVoiceSettings,
    onSuccess: (settings) => client.setQueryData(voiceSettingsQuery.queryKey, settings),
  })

  const copyUrl = (url: string) => {
    void navigator.clipboard.writeText(url).then(() => {
      setCopiedUrl(url)
      setTimeout(() => setCopiedUrl((current) => (current === url ? '' : current)), 1500)
    })
  }

  return (
    <section className="py-4">
      <h1 className="text-lg font-semibold text-ink">Voice</h1>
      <p className="mt-0.5 text-[13px] text-ink-2">
        Talk with Jaz while your thread’s selected agent works in the background.
      </p>

      {query.isPending ? (
        <Skeleton className="mt-4 h-48" />
      ) : query.isError ? (
        <div className="mt-4">
          <EmptyState title="Couldn't load voice settings">{query.error.message}</EmptyState>
        </div>
      ) : (
        <SettingsCard className="mt-4 divide-y divide-border px-4">
          <div className="flex min-h-16 items-center justify-between gap-4 py-3">
            <div>
              <p className="text-[13px] font-medium text-ink">Provider</p>
              <p className="mt-0.5 text-[12px] text-ink-2">Handles listening and speaking.</p>
            </div>
            <Select
              aria-label="Voice provider"
              value={query.data.provider}
              disabled={save.isPending}
              options={query.data.providers.filter((provider) => provider.available || provider.id === 'claude' || provider.id === query.data.provider).map((provider) => ({
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
          {query.data.provider === 'claude' && !query.data.providers.find((provider) => provider.id === 'claude')?.available ? (
            <div className="flex items-center justify-between gap-4 py-3 text-[12px] text-ink-2">
              <p>{query.data.providers.find((provider) => provider.id === 'claude')?.reason}</p>
              {window.jaz?.claudeVoice ? (
                <div className="flex shrink-0 items-center gap-2">
                  <a href="https://claude.ai/login" target="_blank" rel="noreferrer" className="text-ink underline">Sign in</a>
                  <Button size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}>Refresh</Button>
                </div>
              ) : null}
            </div>
          ) : null}
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
      )}

      {clientRuntime.assistant ? (
        <div className="mt-6">
          <h2 className="text-sm font-semibold text-ink">“Hey Jaz” & Mac Shortcuts</h2>
          <p className="mt-0.5 text-[12px] text-ink-2">
            Trigger Jaz from Apple Vocal Shortcuts, Shortcuts, or macOS automations using <code className="font-mono text-ink">jaz://assistant</code>.
          </p>
          <SettingsCard className="mt-3 divide-y divide-border px-4">
            <div className="flex min-h-16 items-center justify-between gap-4 py-3">
              <div>
                <p className="text-[13px] font-medium text-ink">Default trigger behavior</p>
                <p className="mt-0.5 text-[12px] text-ink-2">Switch what <code className="font-mono text-ink">jaz://assistant</code> runs without editing Shortcuts.</p>
              </div>
              <Select
                aria-label="Hey Jaz default behavior"
                value={wakeMode}
                options={WAKE_MODES}
                onChange={(value) => setWakeMode(value as AssistantWakeMode)}
              />
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 py-3">
              <div>
                <p className="text-[13px] font-medium text-ink">Try both modes</p>
                <p className="mt-0.5 text-[12px] text-ink-2">Test Mode A and Mode B right away.</p>
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" onClick={() => clientRuntime.assistant?.trigger({ mode: 'dictate' })}>
                  Test Quick Dictate
                </Button>
                <Button size="sm" variant="secondary" onClick={() => clientRuntime.assistant?.trigger({ mode: 'voice' })}>
                  Test Live Voice
                </Button>
              </div>
            </div>

            <div className="space-y-2 py-3">
              <p className="text-[13px] font-medium text-ink">Automation URLs</p>
              <div className="space-y-1.5">
                {AUTOMATION_LINKS.map((item) => (
                  <div key={item.url} className="flex items-center justify-between gap-3 rounded-lg bg-surface-2/60 px-2.5 py-1.5">
                    <div className="min-w-0">
                      <p className="text-[12px] font-medium text-ink">{item.label}</p>
                      <p className="truncate font-mono text-[11px] text-ink-2">{item.url}</p>
                    </div>
                    <Button size="sm" variant="ghost" onClick={() => copyUrl(item.url)} aria-label={`Copy ${item.label}`}>
                      {copiedUrl === item.url ? <Check size={13} /> : <Copy size={13} />}
                    </Button>
                  </div>
                ))}
              </div>
            </div>

            <div className="space-y-1.5 py-3 text-[12px] text-ink-2">
              <p className="text-[13px] font-medium text-ink">Mac Vocal Shortcuts setup</p>
              <ol className="list-decimal space-y-1 pl-4">
                <li>
                  In <strong>Shortcuts</strong>, create a shortcut with <strong>Run Shell Script</strong> running{' '}
                  <code className="font-mono text-ink">open -g "jaz://assistant"</code> (keeps your current app focused for Quick Dictate) or <strong>URL</strong> + <strong>Open URLs</strong>.
                </li>
                <li>
                  For Shortcuts-driven dictation, use <strong>Dictate Text</strong> → <strong>URL Encode</strong> → <strong>Run Shell Script</strong>:{' '}
                  <code className="font-mono text-ink">open -g "jaz://assistant?text=$1"</code>.
                </li>
                <li>
                  Open <strong>System Settings → Accessibility → Speech → Vocal Shortcuts → Add Action</strong>, pick your shortcut, and train <strong>“Hey Jaz”</strong>.
                </li>
              </ol>
            </div>
          </SettingsCard>
        </div>
      ) : null}
    </section>
  )
}
