import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder, History, LayoutDashboard, Lock, RotateCcw, UserCheck, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/toast'
import {
  appVersionsQuery,
  patchApp,
  rollbackApp,
  type AppVisibility,
  type JazApp,
} from '@/lib/api/apps'
import { addBoardTiles, boardsQuery } from '@/lib/api/boards'
import { orgMembersQuery } from '@/lib/api/cloudMembers'
import { spacesQuery } from '@/lib/api/cloudSpaces'
import { useCloudSession } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

const VISIBILITY_OPTIONS: { id: AppVisibility; label: string; Icon: typeof Lock }[] = [
  { id: 'private', label: 'Private', Icon: Lock },
  { id: 'selected', label: 'Selected', Icon: UserCheck },
  { id: 'space', label: 'Space', Icon: Folder },
  { id: 'team', label: 'Team (Org)', Icon: Users },
]

export function AppShareModal({
  open,
  onClose,
  app,
}: {
  open: boolean
  onClose: () => void
  app: JazApp
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const cloud = useCloudSession()
  const isCloud = cloud.active

  const [title, setTitle] = useState(app.title)
  const [description, setDescription] = useState(app.description ?? '')
  const [visibility, setVisibility] = useState<AppVisibility>(app.visibility)
  const [spaceId, setSpaceId] = useState(app.space_id ?? '')
  const [memberIds, setMemberIds] = useState<string[]>(app.members?.map((m) => m.user_id) ?? [])
  const [pinBoardId, setPinBoardId] = useState('')

  useEffect(() => {
    if (!open) return
    setTitle(app.title)
    setDescription(app.description ?? '')
    setVisibility(app.visibility)
    setSpaceId(app.space_id ?? '')
    setMemberIds(app.members?.map((m) => m.user_id) ?? [])
    setPinBoardId('')
  }, [open, app])

  const spaces = useQuery({ ...spacesQuery, enabled: open && isCloud })
  const members = useQuery({ ...orgMembersQuery(), enabled: open && isCloud && visibility === 'selected' })
  // A board holds no boards and has no versions of its own.
  const isBoard = app.kind === 'board'
  const boards = useQuery({ ...boardsQuery, enabled: open && !isBoard })
  const versions = useQuery({ ...appVersionsQuery(app.id), enabled: open && !isBoard })

  const saveMutation = useMutation({
    mutationFn: () =>
      patchApp(app.id, {
        title: title.trim(),
        description: description.trim(),
        visibility: isCloud ? visibility : 'private',
        space_id: isCloud && visibility === 'space' ? spaceId || null : null,
        member_user_ids: isCloud && visibility === 'selected' ? memberIds : undefined,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast(isBoard ? 'Updated board' : 'Updated app settings')
      onClose()
    },
  })

  const pinMutation = useMutation({
    mutationFn: () => addBoardTiles(pinBoardId, { app_ids: [app.id] }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.appBoard(pinBoardId) })
      toast('Pinned app to board')
      setPinBoardId('')
    },
  })

  const rollbackMutation = useMutation({
    mutationFn: (version: number) => rollbackApp(app.id, version),
    onSuccess: ({ app: updated }) => {
      void queryClient.invalidateQueries({ queryKey: keys.appsRoot })
      toast(`Rolled back to v${updated.current_version}`)
    },
  })

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="md"
      title={`${isCloud ? 'Share & Manage' : 'Manage'} · ${app.title}`}
      description={isBoard
        ? isCloud ? 'Rename the board and choose who can open it.' : 'Rename the board.'
        : isCloud
          ? 'Control authenticated Org/Space/Selected/Private access, pin to a board, or roll back immutable versions.'
          : 'Rename, pin to a board, or roll back immutable versions.'}
      footer={
        <>
          <div className="min-w-0 truncate text-[12px] text-danger">
            {saveMutation.isError ? (saveMutation.error as Error).message : ''}
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button variant="ghost" size="md" onClick={onClose}>
              Close
            </Button>
            {app.can_edit !== false ? (
              <Button
                variant="primary"
                size="md"
                disabled={saveMutation.isPending || !title.trim() || (isCloud && visibility === 'space' && !spaceId)}
                onClick={() => saveMutation.mutate()}
              >
                {saveMutation.isPending ? 'Saving…' : 'Save changes'}
              </Button>
            ) : null}
          </div>
        </>
      }
    >
      <div className="space-y-4 text-[13px]">
        <div className="grid gap-2.5 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-[12px] font-medium text-ink-2">Title</label>
            <Input value={title} disabled={app.can_edit === false} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div>
            <label className="mb-1 block text-[12px] font-medium text-ink-2">Description</label>
            <Input value={description} disabled={app.can_edit === false} onChange={(e) => setDescription(e.target.value)} />
          </div>
        </div>

        {isCloud ? (
          <div>
            <label className="mb-1.5 block text-[12px] font-medium text-ink-2">Access</label>
            <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
              {VISIBILITY_OPTIONS.map(({ id, label, Icon }) => (
                <button
                  key={id}
                  type="button"
                  disabled={app.can_edit === false}
                  onClick={() => setVisibility(id)}
                  className={`flex cursor-pointer items-center gap-1.5 rounded-control border px-2.5 py-2 text-[12px] font-medium ${
                    visibility === id
                      ? 'border-primary bg-primary-soft text-ink'
                      : 'border-border bg-bg text-ink-2 hover:bg-surface'
                  }`}
                >
                  <Icon size={13} />
                  <span className="truncate">{label}</span>
                </button>
              ))}
            </div>

            {visibility === 'space' ? (
              <select
                value={spaceId}
                disabled={app.can_edit === false}
                onChange={(e) => setSpaceId(e.target.value)}
                className="mt-2 w-full rounded-control border border-border bg-bg px-2.5 py-1.5 text-[12.5px] text-ink"
              >
                <option value="">Select a Space…</option>
                {(spaces.data ?? []).map((sp) => (
                  <option key={sp.id} value={sp.id}>{sp.name}</option>
                ))}
              </select>
            ) : null}

            {visibility === 'selected' ? (
              <div className="mt-2 max-h-28 space-y-1 overflow-y-auto rounded-control border border-border bg-surface p-2">
                {(members.data ?? []).map((m) => (
                  <label key={m.id} className="flex cursor-pointer items-center gap-2 text-[12px]">
                    <input
                      type="checkbox"
                      disabled={app.can_edit === false}
                      checked={memberIds.includes(m.id)}
                      onChange={() =>
                        setMemberIds((cur) =>
                          cur.includes(m.id) ? cur.filter((id) => id !== m.id) : [...cur, m.id],
                        )
                      }
                      className="accent-primary"
                    />
                    <span className="truncate font-medium text-ink">{m.display_name || m.email}</span>
                  </label>
                ))}
              </div>
            ) : null}
          </div>
        ) : null}

        {isBoard ? null : (
          <div className="rounded-control border border-border bg-surface p-3">
            <div className="mb-1.5 flex items-center gap-1.5 text-[12px] font-medium text-ink">
              <LayoutDashboard size={13} />
              Pin to board
            </div>
            <div className="flex items-center gap-2">
              <select
                value={pinBoardId}
                onChange={(e) => setPinBoardId(e.target.value)}
                className="min-w-0 flex-1 rounded-control border border-border bg-bg px-2.5 py-1.5 text-[12.5px] text-ink"
              >
                <option value="">Choose a board…</option>
                {(boards.data ?? []).map((b) => (
                  <option key={b.id} value={b.id}>{b.title}</option>
                ))}
              </select>
              <Button
                variant="secondary"
                size="sm"
                disabled={!pinBoardId || pinMutation.isPending}
                onClick={() => pinMutation.mutate()}
              >
                {pinMutation.isPending ? 'Pinning…' : 'Pin'}
              </Button>
            </div>
          </div>
        )}

        {(versions.data ?? []).length > 0 ? (
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[12px] font-medium text-ink-2">
              <History size={13} />
              Version history
            </div>
            <ul className="max-h-36 divide-y divide-border rounded-control border border-border bg-bg">
              {(versions.data ?? []).map((v) => {
                const isCurrent = v.version === app.current_version
                return (
                  <li key={v.version} className="flex items-center justify-between gap-2 px-3 py-1.5 text-[12px]">
                    <div className="min-w-0">
                      <span className="font-medium text-ink">v{v.version}</span>
                      <span className="ml-2 font-mono text-[11px] text-ink-3">{v.bundle_sha256.slice(0, 12)}</span>
                      {isCurrent ? (
                        <span className="ml-2 rounded-full bg-primary-soft px-1.5 py-px text-[10px] font-medium text-primary">
                          current
                        </span>
                      ) : null}
                    </div>
                    {!isCurrent && app.can_edit !== false ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={rollbackMutation.isPending}
                        onClick={() => rollbackMutation.mutate(v.version)}
                      >
                        <RotateCcw size={11} />
                        Rollback
                      </Button>
                    ) : null}
                  </li>
                )
              })}
            </ul>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
