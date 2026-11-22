import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Lock, ShieldOff, ShieldCheck, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { isCloudThread } from '@/lib/api/cloudClient'
import { spacesQuery, updateThreadUserState } from '@/lib/api/cloudSpaces'
import type { Session } from '@/lib/api/types'
import { useHasCloud } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

// Compact visibility badge (`Shared in <Space>` vs `Private`) plus an explicit
// Personal Sources opt-in switch (default OFF) on shared Space threads.
export function CloudThreadControls({ session }: { session: Session }) {
  const hasCloud = useHasCloud()
  const isCloudSession = session.backend === 'cloud' || isCloudThread(session.id)
  const queryClient = useQueryClient()
  const toast = useToast()
  const spaces = useQuery({ ...spacesQuery, enabled: isCloudSession && hasCloud })
  const isShared = Boolean(session.space_id || session.visibility === 'space')
  const spaceName = session.space_id
    ? spaces.data?.find((space) => space.id === session.space_id)?.name ?? 'Space'
    : 'Space'

  const [personalSources, setPersonalSources] = useState(Boolean(session.personal_sources_enabled))
  useEffect(() => {
    setPersonalSources(Boolean(session.personal_sources_enabled))
  }, [session.id, session.personal_sources_enabled])

  const togglePersonal = useMutation({
    mutationFn: (enabled: boolean) => updateThreadUserState(session.id, { personal_sources_enabled: enabled }),
    onMutate: (enabled) => {
      const previous = personalSources
      setPersonalSources(enabled)
      return { previous }
    },
    onSuccess: (updated) => {
      setPersonalSources(Boolean(updated.personal_sources_enabled))
      void queryClient.invalidateQueries({ queryKey: keys.session(session.id) })
      void queryClient.invalidateQueries({ queryKey: keys.sessionMessages(session.id) })
    },
    onError: (error: Error, _enabled, context) => {
      if (context) setPersonalSources(context.previous)
      toast(`Could not update Personal Sources: ${error.message}`, 'danger')
    },
  })

  if (!isCloudSession) return null

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <span
        className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ${
          isShared ? 'bg-primary-soft text-primary' : 'bg-surface-2 text-ink-2'
        }`}
        title={isShared ? `Shared in ${spaceName}` : 'Private to you'}
      >
        {isShared ? <Users size={11} /> : <Lock size={11} />}
        <span className="max-w-[140px] truncate">{isShared ? `Shared in ${spaceName}` : 'Private'}</span>
      </span>

      {isShared ? (
        <label
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-full bg-surface px-2 py-0.5 text-[11px] text-ink-2 ring-1 ring-border/70 transition-colors duration-150 hover:text-ink"
          title={
            personalSources
              ? 'Personal connectors are enabled for your turns in this shared thread'
              : 'Personal connectors (mailbox, WhatsApp, paired machines) are isolated in this shared thread'
          }
        >
          {personalSources ? (
            <ShieldCheck size={11} className="text-ok" />
          ) : (
            <ShieldOff size={11} className="text-ink-3" />
          )}
          <span className="whitespace-nowrap">Personal Sources</span>
          <Switch
            checked={personalSources}
            disabled={togglePersonal.isPending}
            aria-label="Personal Sources"
            onChange={(checked) => togglePersonal.mutate(checked)}
            className="scale-[0.82]"
          />
        </label>
      ) : null}
    </div>
  )
}
