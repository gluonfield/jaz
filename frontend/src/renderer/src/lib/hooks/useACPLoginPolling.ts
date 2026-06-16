import { useCallback, useEffect, useRef, useState } from 'react'
import { getACPAuthLogin } from '@/lib/api/settings'
import type { ACPAuthLogin } from '@/lib/api/types'

// Tracks in-flight ACP sign-in jobs and polls them to completion. Owns the
// per-job state and the interval; callers supply `onResolved`, which runs once
// a job stops running (its work — invalidating queries, enabling the agent —
// is the only thing that differs between the onboarding and settings screens).
// `onResolved` is read through a ref, so it always sees the caller's latest
// closure without re-arming the interval.
export function useACPLoginPolling(onResolved: (job: ACPAuthLogin) => void) {
  const [loginJobs, setLoginJobs] = useState<Record<string, ACPAuthLogin>>({})
  const onResolvedRef = useRef(onResolved)
  onResolvedRef.current = onResolved

  useEffect(() => {
    const running = Object.values(loginJobs).filter((job) => job.status === 'running')
    if (running.length === 0) return
    const timer = window.setInterval(() => {
      for (const job of running) {
        void getACPAuthLogin(job.id).then((next) => {
          setLoginJobs((current) => ({ ...current, [next.agent]: next }))
          if (next.status !== 'running') onResolvedRef.current(next)
        })
      }
    }, 1000)
    return () => window.clearInterval(timer)
  }, [loginJobs])

  const trackLoginJob = useCallback((job: ACPAuthLogin) => {
    setLoginJobs((current) => ({ ...current, [job.agent]: job }))
  }, [])

  const forgetLoginJob = useCallback((agent: string) => {
    setLoginJobs((current) => {
      if (!(agent in current)) return current
      const next = { ...current }
      delete next[agent]
      return next
    })
  }, [])

  return { loginJobs, trackLoginJob, forgetLoginJob }
}
