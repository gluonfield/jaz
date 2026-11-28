import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ImageUp } from 'lucide-react'
import { type FormEvent, useRef, useState } from 'react'
import { ORG_ICON_PRESETS, OrgIconMark } from '@/components/cloud/OrgIconMark'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useToast } from '@/components/ui/toast'
import { adminOrgQuery, MAX_ORG_ICON_BYTES, updateAdminOrg } from '@/lib/api/cloudOrg'
import { keys } from '@/lib/query/keys'

const ICON_CHOICE =
  'grid size-8 cursor-pointer place-items-center rounded-lg text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink disabled:cursor-default disabled:opacity-50 aria-pressed:bg-ink aria-pressed:text-bg'

function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error('Could not read the image'))
    reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ''))
    reader.readAsDataURL(file)
  })
}

export function OrgIdentitySection() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const fail = (error: Error) => toast(error.message, 'danger')
  const org = useQuery(adminOrgQuery)
  const [draftName, setDraftName] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const save = useMutation({
    mutationFn: updateAdminOrg,
    onSuccess: (updated) => {
      queryClient.setQueryData(keys.cloudAdminOrg, updated)
      setDraftName(null)
      void queryClient.invalidateQueries({ queryKey: keys.cloudOrgIdentity })
    },
    onError: fail,
  })

  const upload = async (file: File | undefined) => {
    if (!file) return
    if (file.size > MAX_ORG_ICON_BYTES) {
      toast('Icon image must be 256 KB or smaller', 'danger')
      return
    }
    try {
      save.mutate({ icon: { kind: 'image', data_base64: await readBase64(file) } })
    } catch (error) {
      fail(error as Error)
    }
  }

  const name = draftName ?? org.data?.name ?? ''
  const nameChanged = Boolean(org.data && name.trim() && name.trim() !== org.data.name)
  const submitName = (event: FormEvent) => {
    event.preventDefault()
    if (nameChanged && !save.isPending) save.mutate({ name: name.trim() })
  }

  return (
    <SettingsCard className="divide-y divide-border">
      {org.data ? (
        <>
          <form onSubmit={submitName} className="flex items-center gap-3 px-3 py-3">
            <OrgIconMark name={org.data.name} icon={org.data.icon} className="size-9 rounded-[10px] text-[16px]" />
            <Input
              aria-label="Organization name"
              value={name}
              maxLength={80}
              onChange={(event) => setDraftName(event.target.value)}
              className="min-w-0 flex-1"
            />
            {nameChanged ? (
              <Button type="submit" variant="primary" size="sm" disabled={save.isPending}>
                Save
              </Button>
            ) : null}
          </form>
          <div role="group" aria-label="Organization icon" className="flex flex-wrap items-center gap-1 px-3 py-2.5">
            <button
              type="button"
              title="Initial"
              aria-label="Initial"
              aria-pressed={org.data.icon.kind === 'none'}
              disabled={save.isPending}
              onClick={() => save.mutate({ icon: { kind: 'none' } })}
              className={`${ICON_CHOICE} text-[13px] font-semibold uppercase`}
            >
              {org.data.name.charAt(0)}
            </button>
            {Object.entries(ORG_ICON_PRESETS).map(([preset, Icon]) => (
              <button
                key={preset}
                type="button"
                title={preset}
                aria-label={preset}
                aria-pressed={org.data.icon.kind === 'preset' && org.data.icon.preset === preset}
                disabled={save.isPending}
                onClick={() => save.mutate({ icon: { kind: 'preset', preset } })}
                className={ICON_CHOICE}
              >
                <Icon size={16} />
              </button>
            ))}
            <button
              type="button"
              title="Upload a PNG or JPEG, up to 256 KB"
              aria-label="Upload image"
              aria-pressed={org.data.icon.kind === 'image'}
              disabled={save.isPending}
              onClick={() => fileInput.current?.click()}
              className={ICON_CHOICE}
            >
              <ImageUp size={16} />
            </button>
            <input
              ref={fileInput}
              type="file"
              accept="image/png,image/jpeg"
              hidden
              onChange={(event) => {
                void upload(event.target.files?.[0])
                event.target.value = ''
              }}
            />
          </div>
        </>
      ) : (
        <p className="px-3 py-6 text-center text-[13px] text-ink-3">{org.error?.message ?? 'Loading…'}</p>
      )}
    </SettingsCard>
  )
}
