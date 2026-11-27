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
import { AdminSection } from './AdminSection'

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
    <AdminSection title="Organization" description="The name and icon members see in the workspace switcher.">
      <SettingsCard className="divide-y divide-border">
        {org.data ? (
          <>
            <form onSubmit={submitName} className="flex items-center gap-2 px-3 py-2.5">
              <label htmlFor="org-name" className="w-14 shrink-0 text-[13px] font-medium text-ink">
                Name
              </label>
              <Input
                id="org-name"
                value={name}
                maxLength={80}
                onChange={(event) => setDraftName(event.target.value)}
                className="min-w-0 flex-1"
              />
              <Button type="submit" variant="primary" size="sm" disabled={!nameChanged || save.isPending}>
                Save
              </Button>
            </form>
            <div className="flex items-start gap-2 px-3 py-2.5">
              <p className="w-14 shrink-0 pt-1.5 text-[13px] font-medium text-ink">Icon</p>
              <div role="group" aria-label="Organization icon" className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
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
                {org.data.icon.kind === 'image' ? (
                  <OrgIconMark name={org.data.name} icon={org.data.icon} className="mx-1 size-8 rounded-lg" />
                ) : null}
                <Button size="sm" disabled={save.isPending} onClick={() => fileInput.current?.click()}>
                  <ImageUp size={13} />
                  Upload image
                </Button>
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
                <p className="basis-full pt-1 text-[12px] text-ink-3">PNG or JPEG, 16 to 512 pixels square, up to 256 KB.</p>
              </div>
            </div>
          </>
        ) : (
          <p className="px-3 py-6 text-center text-[13px] text-ink-3">{org.error?.message ?? 'Loading…'}</p>
        )}
      </SettingsCard>
    </AdminSection>
  )
}
