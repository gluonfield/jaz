import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { randomAvatar } from '@/lib/bots'
import { useCreateBot } from './useCreateBot'
import { BotWorkspacePicker } from './BotWorkspaceSettings'

export function NewBotDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [name, setName] = useState('')
  const [workspaces, setWorkspaces] = useState<string[]>([])
  const create = useCreateBot()
  const close = () => {
    setName('')
    setWorkspaces([])
    onClose()
  }
  const submit = () => create.mutate(
    { name: name.trim(), avatar: randomAvatar(), workspaces },
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
      {open ? <BotWorkspacePicker value={workspaces} onChange={setWorkspaces} /> : null}
    </Modal>
  )
}
