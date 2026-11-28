import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { BoardIllustration } from '@/components/boards/BoardIllustration'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { createBoard } from '@/lib/api/boards'
import { keys } from '@/lib/query/keys'

// Creates a board, then opens it with the tile picker up. A new board is
// private; sharing it is the same as sharing any app.
export function BoardModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')

  const create = useMutation({
    mutationFn: () => createBoard(name.trim() || 'Board'),
    onSuccess: (board) => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      onClose()
      void navigate({ to: '/apps/$appId', params: { appId: board.id }, search: { add: true } })
    },
  })

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      title="New board"
      description="An app that holds other apps as tiles."
      footer={
        <>
          <p className="text-[12px] text-danger" role="alert">
            {create.isError ? create.error.message : ''}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="md" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" size="md" disabled={create.isPending} onClick={() => create.mutate()}>
              {create.isPending ? 'Creating…' : 'Create board'}
            </Button>
          </div>
        </>
      }
    >
      <div className="space-y-5">
        <BoardIllustration />
        <p className="text-[13px] leading-relaxed text-ink-2">
          Put a loop on a board and every run republishes its app there with what it just gathered.
          Pin any other app beside it. Drag and resize tiles; your layout always wins.
        </p>
        <label className="block">
          <span className="mb-1.5 block text-[12px] font-medium text-ink-2">Name</span>
          <Input
            type="text"
            value={name}
            autoFocus
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !create.isPending) create.mutate()
            }}
            placeholder="Mission control"
          />
        </label>
      </div>
    </Modal>
  )
}
