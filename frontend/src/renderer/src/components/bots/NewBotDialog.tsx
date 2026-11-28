import { useState } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { randomAvatar } from '@/lib/bots'
import { useAgentModelCatalog } from '@/lib/hooks/useAgentModelCatalog'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { useCreateBot } from './useCreateBot'
import { BotWorkspacePicker } from './BotWorkspaceSettings'

export function NewBotDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [name, setName] = useState('')
  const [workspaces, setWorkspaces] = useState<string[]>([])
  const [prompt, setPrompt] = useState('')
  const [profileId, setProfileId] = useState('')
  const [model, setModel] = useState('')
  const [effort, setEffort] = useState('')
  const create = useCreateBot()
  const catalog = useAgentModelCatalog({
    enabled: open,
    agent: profileId,
    provider: profileId,
    model,
    effort,
  })
  const close = () => {
    setName('')
    setWorkspaces([])
    setPrompt('')
    setProfileId('')
    setModel('')
    setEffort('')
    onClose()
  }
  const submit = () => {
    const resolved = catalog.usingCloud && catalog.agent ? catalog.pickAgent(catalog.agent) : null
    create.mutate(
      {
        name: name.trim(),
        avatar: randomAvatar(),
        workspaces: catalog.usingCloud ? [] : workspaces,
        prompt: prompt.trim(),
        ...(resolved
          ? {
              agent: resolved.resolvedAgent || 'codex',
              model: catalog.model || undefined,
              reasoning_effort: catalog.effort || undefined,
            }
          : {}),
      },
      { onSuccess: close },
    )
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title="New bot"
      size="sm"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button variant="ghost" onClick={close}>Cancel</Button>
          <Button variant="primary" disabled={!name.trim() || create.isPending} onClick={submit}>Create</Button>
        </div>
      }
    >
      <Input autoFocus value={name} placeholder="Name" onChange={(event) => setName(event.target.value)} />
      <label className="mt-4 flex flex-col gap-1.5 text-[13px] text-ink">
        <span className="font-medium">Role and scope</span>
        <textarea aria-label="Role and scope" value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={3} placeholder="What should this bot do, and what should it leave to others?" className="w-full resize-y rounded-control bg-bg px-3 py-2 text-[13px] text-ink ring-1 ring-border outline-none placeholder:text-ink-3 focus:ring-primary" />
        <span className="text-[12px] text-ink-3">Saved as this bot's instructions. Project knowledge comes from the linked folders; private memory stays in the bot's home.</span>
      </label>
      {open && catalog.usingCloud ? (
        <div className="mt-4 flex items-center justify-between gap-3">
          <span className="text-[13px] font-medium text-ink">Model</span>
          {catalog.ready && catalog.agents.length === 0 ? (
            <span className="text-[12px] text-ink-3">No cloud models configured</span>
          ) : (
            <ModelSelect
              agent={{
                value: catalog.agent,
                options: catalog.agents,
                items: catalog.agentItems,
                onChange: (nextProfileId) => {
                  const next = catalog.pickAgent(nextProfileId)
                  setProfileId(next.agent)
                  setModel(next.model)
                  setEffort(next.effort)
                },
              }}
              value={catalog.model}
              suggestions={catalog.modelSuggestions}
              effort={catalog.effort}
              effortOptions={pickerEffortOptions(catalog.effortOptions)}
              loading={catalog.loading}
              disabled={create.isPending}
              placement="below"
              align="end"
              onChange={(next) => {
                setModel(next.model)
                setEffort(next.effort)
              }}
            />
          )}
        </div>
      ) : null}
      {open && !catalog.usingCloud ? <BotWorkspacePicker value={workspaces} onChange={setWorkspaces} /> : null}
    </Modal>
  )
}
