import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import type { Bot } from '@/lib/api/types'
import { useUpdateBot } from './useUpdateBot'

export function BotRoleField({ value, onChange, disabled }: { value: string; onChange: (value: string) => void; disabled?: boolean }) {
  return (
    <label className="mt-4 flex flex-col gap-1.5 text-[13px] text-ink">
      <span className="font-medium">Role and scope</span>
      <textarea aria-label="Role and scope" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} rows={3} placeholder="What should this bot do, and what should it leave to others?" className="w-full resize-y rounded-control bg-bg px-3 py-2 text-[13px] text-ink ring-1 ring-border outline-none placeholder:text-ink-3 focus:ring-primary" />
      <span className="text-[12px] text-ink-3">Saved as this bot's instructions. Project knowledge comes from the linked folders; private memory stays in the bot's home.</span>
    </label>
  )
}

export function BotRoleSettings({ bot }: { bot: Bot }) {
  const [prompt, setPrompt] = useState(bot.prompt ?? '')
  const update = useUpdateBot(bot.id)
  return (
    <div>
      <BotRoleField value={prompt} onChange={setPrompt} disabled={update.isPending} />
      <div className="mt-2 flex justify-end">
        <Button disabled={update.isPending || prompt.trim() === (bot.prompt ?? '')} onClick={() => update.mutate({ prompt: prompt.trim() })}>Save role</Button>
      </div>
    </div>
  )
}
