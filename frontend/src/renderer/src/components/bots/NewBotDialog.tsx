import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { ModelSelect } from '@/components/session/ModelSelect'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { cloudModelProfilesQuery, resolveCloudModelSelection } from '@/lib/api/cloudThreads'
import { randomAvatar } from '@/lib/bots'
import { useCloudSession, useIsCloudOnly } from '@/lib/cloudSession'
import { pickerEffortOptions } from '@/lib/modelPicker'
import { useCreateBot } from './useCreateBot'
import { BotWorkspacePicker } from './BotWorkspaceSettings'
import { BotRoleField } from './BotRoleSettings'

export function NewBotDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const cloud = useCloudSession()
  const cloudOnly = useIsCloudOnly()
  const usingCloudProfiles = cloudOnly || cloud.active
  const cloudProfilesQuery = useQuery({ ...cloudModelProfilesQuery, enabled: open && usingCloudProfiles && cloud.active })
  const cloudProfiles = useMemo(
    () => (usingCloudProfiles ? (cloudProfilesQuery.data ?? []) : []),
    [cloudProfilesQuery.data, usingCloudProfiles],
  )
  const [name, setName] = useState('')
  const [workspaces, setWorkspaces] = useState<string[]>([])
  const [prompt, setPrompt] = useState('')
  const [profileId, setProfileId] = useState('')
  const [model, setModel] = useState('')
  const [effort, setEffort] = useState('')
  const create = useCreateBot()
  const cloudSelection = useMemo(
    () =>
      usingCloudProfiles
        ? resolveCloudModelSelection(cloudProfiles, { profileId, model, effort })
        : null,
    [cloudProfiles, effort, model, profileId, usingCloudProfiles],
  )
  const close = () => {
    setName('')
    setWorkspaces([])
    setPrompt('')
    setProfileId('')
    setModel('')
    setEffort('')
    onClose()
  }
  const submit = () => create.mutate(
    {
      name: name.trim(),
      avatar: randomAvatar(),
      workspaces: usingCloudProfiles ? [] : workspaces,
      prompt: prompt.trim(),
      ...(usingCloudProfiles && cloudSelection?.profile
        ? {
            agent: cloudSelection.profile.agent || 'codex',
            model: cloudSelection.model || undefined,
            reasoning_effort: cloudSelection.effort || undefined,
          }
        : {}),
    },
    { onSuccess: close },
  )

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
      <BotRoleField value={prompt} onChange={setPrompt} />
      {open && usingCloudProfiles ? (
        <div className="mt-4 flex items-center justify-between gap-3">
          <span className="text-[13px] font-medium text-ink">Model</span>
          {cloudProfilesQuery.isSuccess && (cloudSelection?.agents.length ?? 0) === 0 ? (
            <span className="text-[12px] text-ink-3">No cloud models configured</span>
          ) : (
            <ModelSelect
              agent={{
                value: cloudSelection?.profileId ?? '',
                options: cloudSelection?.agents ?? [],
                items: cloudSelection?.agentItems,
                onChange: (nextProfileId) => {
                  const next = resolveCloudModelSelection(cloudProfiles, { profileId: nextProfileId })
                  setProfileId(nextProfileId)
                  setModel(next.model)
                  setEffort(next.effort)
                },
              }}
              value={cloudSelection?.model ?? ''}
              suggestions={cloudSelection?.modelSuggestions ?? []}
              effort={cloudSelection?.effort ?? ''}
              effortOptions={pickerEffortOptions(cloudSelection?.effortOptions ?? [])}
              loading={cloudProfilesQuery.isPending}
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
      {open && !usingCloudProfiles ? <BotWorkspacePicker value={workspaces} onChange={setWorkspaces} /> : null}
    </Modal>
  )
}
