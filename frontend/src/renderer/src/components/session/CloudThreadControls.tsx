import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Folder, Lock, ShieldCheck, ShieldOff, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { isCloudApiSession } from '@/lib/api/client'
import { spacesQuery, updateThreadUserState } from '@/lib/api/cloudSpaces'
import type { Session } from '@/lib/api/types'
import { useHasCloud, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

// Compact visibility/project badge (`Shared in <Project>`, `<Project>`, or `Private`)
// plus an explicit Personal Sources opt-in switch (default OFF) on shared project threads.
export function CloudThreadControls({ session }: { session: Session }) {
  const hasCloud = useHasCloud()
  const isViewer = useIsCloudViewer()
  const isCloudSession = session.backend === 'cloud' || isCloudApiSession()
  const queryClient = useQueryClient()
  const toast = useToast()
  const spaces = useQuery({ ...spacesQuery, enabled: isCloudSession && hasCloud })
  const isShared = session.visibility === 'space'
  const spaceName = session.space_id
    ? spaces.data?.find((space) => space.id === session.space_id)?.name ?? 'Project'
    : 'Project'

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

  const pillClass = `inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors ${
    isShared ? 'bg-primary-soft text-primary hover:bg-primary-soft/80' : 'bg-surface-2 text-ink-2 hover:text-ink'
  }`

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      {session.space_id ? (
        <Link
          to="/spaces/$spaceId"
          params={{ spaceId: session.space_id }}
          className={pillClass}
          title={isShared ? `Shared in ${spaceName}` : `In private project ${spaceName}`}
        >
          {isShared ? <Users size={11} /> : <Folder size={11} />}
          <span className="max-w-[140px] truncate">{isShared ? `Shared in ${spaceName}` : spaceName}</span>
        </Link>
      ) : (
        <span className={pillClass} title="Private to you">
          <Lock size={11} />
          <span>Private</span>
        </span>
      )}

      {isShared && !isViewer ? (
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
