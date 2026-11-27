import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { ASSISTANT_URLS, type AssistantWakeMode } from '@shared/assistant'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { useAssistantWakeMode } from '@/lib/assistant/settings'
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
  { label: 'Default ("Hey Jaz")', url: ASSISTANT_URLS.default },
  { label: 'Mode A — Live voice', url: ASSISTANT_URLS.voice },
  { label: 'Mode B — Quick dictate', url: ASSISTANT_URLS.dictate },
  { label: 'Mode B — Send dictated text', url: `${ASSISTANT_URLS.sendExample}Hello` },
]

export function HeyJazSettings() {
  const assistant = clientRuntime.assistant
  const [wakeMode, setWakeMode] = useAssistantWakeMode()
  const [copiedUrl, setCopiedUrl] = useState('')

  if (!assistant) return null

  const copyUrl = (url: string) => {
    void navigator.clipboard.writeText(url).then(() => {
      setCopiedUrl(url)
      setTimeout(() => setCopiedUrl((current) => (current === url ? '' : current)), 1500)
    })
  }

  return (
    <div className="mt-6">
      <h2 className="text-sm font-semibold text-ink">“Hey Jaz” & Mac Shortcuts</h2>
      <p className="mt-0.5 text-[12px] text-ink-2">
        Trigger Jaz from Apple Vocal Shortcuts, Shortcuts, or macOS automations using <code className="font-mono text-ink">jaz://assistant</code>.
      </p>
      <SettingsCard className="mt-3 divide-y divide-border px-4">
        <div className="flex min-h-16 items-center justify-between gap-4 py-3">
          <div>
            <p className="text-[13px] font-medium text-ink">Default trigger behavior</p>
            <p className="mt-0.5 text-[12px] text-ink-2">
              Switch what <code className="font-mono text-ink">jaz://assistant</code> runs without editing Shortcuts.
            </p>
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
            <Button size="sm" variant="secondary" onClick={() => assistant.trigger({ mode: 'dictate' })}>
              Test Quick Dictate
            </Button>
            <Button size="sm" variant="secondary" onClick={() => assistant.trigger({ mode: 'voice' })}>
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
              In <strong>Shortcuts</strong>, add <strong>Run Shell Script</strong> with{' '}
              <code className="font-mono text-ink">open -g "jaz://assistant"</code> (keeps your current app focused for Quick Dictate) or <strong>URL</strong> + <strong>Open URLs</strong>.
            </li>
            <li>
              For Shortcuts-driven dictation, add <strong>Dictate Text</strong> → <strong>URL Encode</strong> → <strong>Run Shell Script</strong> (Pass Input: <em>as arguments</em>):{' '}
              <code className="font-mono text-ink">open -g "jaz://assistant?text=$1"</code>.
            </li>
            <li>
              Open <strong>System Settings → Accessibility → Speech → Vocal Shortcuts → Add Action</strong>, choose your shortcut, and train <strong>“Hey Jaz”</strong>.
            </li>
          </ol>
        </div>
      </SettingsCard>
    </div>
  )
}
