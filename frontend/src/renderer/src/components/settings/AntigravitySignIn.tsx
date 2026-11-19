import { useQuery } from '@tanstack/react-query'
import { LoaderCircle, LogIn } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { AuthLoginStatus } from '@/components/acp/AuthLoginStatus'
import { Button } from '@/components/ui/Button'
import { Select } from '@/components/ui/Select'
import { get } from '@/lib/api/client'
import type { ACPAuthLogin } from '@/lib/api/types'
import { openExternal } from '@/lib/clientRuntime'
import { connectTerminal } from '@/lib/terminalConnection'

const authPath = '/v1/acp/agents/antigravity/auth'
const loginOptions = [
  { value: 'oauth-personal', label: 'Google Account', description: 'Personal Antigravity plan' },
  { value: 'oauth-business', label: 'Google Cloud project', description: 'Gemini Enterprise or pay-as-you-go' },
]

export function AntigravitySignIn({ active, disabled, running, loginJob, onPersonal, onConnected }: {
  active: boolean
  disabled: boolean
  running: boolean
  loginJob?: ACPAuthLogin
  onPersonal: () => void
  onConnected: () => void
}) {
  const [selection, setSelection] = useState('oauth-personal')
  const [cloudOpen, setCloudOpen] = useState(false)
  const [error, setError] = useState('')
  const { data, isError, isFetching, refetch } = useQuery({
    queryKey: ['antigravity', 'login-methods'],
    queryFn: () => get<string[]>(`${authPath}/methods`),
    enabled: active && !running && !cloudOpen,
    retry: false,
  })
  const options = loginOptions.filter(({ value }) => data?.includes(value))
  const selected = options.find(({ value }) => value === selection) ?? options[0]
  const cloud = selected?.value === 'oauth-business'

  useEffect(() => {
    if (!active) setCloudOpen(false)
  }, [active])

  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {options.length > 1 ? (
          <Select
            aria-label="Antigravity sign-in method"
            value={selected?.value ?? selection}
            options={options}
            onChange={setSelection}
            disabled={disabled || running || cloudOpen}
          />
        ) : null}
        <Button variant="primary" size="md" disabled={disabled || running || cloudOpen || isFetching} onClick={() => {
          setError('')
          if (cloud) setCloudOpen(true)
          else onPersonal()
        }}>
          {running || isFetching ? <LoaderCircle size={14} className="animate-spin" /> : <LogIn size={14} />}
          {running ? 'Waiting for sign-in…' : isFetching ? 'Checking sign-in…' : cloud ? 'Continue in Antigravity' : 'Sign in with Google'}
        </Button>
        {cloudOpen ? <Button variant="ghost" size="sm" onClick={() => setCloudOpen(false)}>Cancel</Button> : null}
      </div>
      {active && cloudOpen ? (
        <>
          <p className="text-[12px] text-ink-3">
            Choose Use a Google Cloud project in Antigravity. Open its Google link and paste the code here.
            After choosing your project and region, press Ctrl+D to finish.
          </p>
          <AntigravityAuthTerminal onExit={(error) => {
            setCloudOpen(false)
            setError(error)
            if (!error) onConnected()
          }} />
        </>
      ) : null}
      {isError ? (
        <div className="flex flex-wrap items-center gap-2 text-[12px] text-ink-3">
          Could not load Google Cloud sign-in availability.
          <Button size="sm" variant="ghost" disabled={isFetching} onClick={() => void refetch()}>Retry</Button>
        </div>
      ) : null}
      {error ? <p role="alert" className="text-[12px] text-danger">{error}</p> : null}
      {cloudOpen ? null : <AuthLoginStatus job={loginJob} running={running} />}
    </div>
  )
}

function AntigravityAuthTerminal({ onExit }: { onExit: (error: string) => void }) {
  const hostRef = useRef<HTMLDivElement>(null)
  const onExitRef = useRef(onExit)
  onExitRef.current = onExit
  useEffect(() => {
    const host = hostRef.current
    if (!host) return
    let resolved = false
    const connection = connectTerminal(host, `${authPath}/cloud-terminal`, (event) => {
      if (resolved || event.type === 'ready') return
      resolved = true
      if (event.type === 'exit') {
        onExitRef.current(event.error || (event.code ? 'Antigravity sign-in failed.' : ''))
      } else {
        onExitRef.current(event.type === 'error' ? event.error : 'The Antigravity sign-in terminal closed.')
      }
    }, (link) => {
      if (link.startsWith('https://accounts.google.com/')) openExternal(link)
    })
    return connection?.close
  }, [])

  return <div ref={hostRef} className="jaz-terminal !h-80 overflow-hidden rounded-control bg-[#111318]" />
}
