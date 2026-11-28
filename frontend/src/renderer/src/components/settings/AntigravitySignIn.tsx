import { LogIn } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import type { ACPGoogleCloudLogin } from '@/lib/api/types'

const accounts = [
  { value: 'google', label: 'Google Account (Personal)' },
  { value: 'enterprise', label: 'Google Cloud project (Enterprise)' },
  { value: 'payg', label: 'Google Cloud project (Pay-as-you-go)' },
]
const fieldClass = 'h-8 rounded-full bg-bg px-3 py-0 font-mono text-[12px]'

export function AntigravitySignIn({ disabled, onStartLogin }: {
  disabled: boolean
  onStartLogin: (googleCloud?: ACPGoogleCloudLogin) => void
}) {
  const [account, setAccount] = useState('google')
  const [project, setProject] = useState('')
  const [location, setLocation] = useState('global')
  const payAsYouGo = account === 'payg'
  const cloud = account !== 'google'
  const incomplete = cloud && (project.trim() ? !location.trim() : payAsYouGo)

  return (
    <div className="flex flex-col items-start gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          aria-label="Antigravity account"
          value={account}
          options={accounts}
          onChange={setAccount}
          disabled={disabled}
        />
        <Button
          variant="primary"
          size="md"
          disabled={disabled || incomplete}
          onClick={() => onStartLogin(cloud ? { project, location, pay_as_you_go: payAsYouGo } : undefined)}
        >
          <LogIn size={14} />
          Sign in with Google
        </Button>
      </div>
      {cloud ? (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Input
              aria-label="Google Cloud project ID"
              value={project}
              disabled={disabled}
              onChange={(event) => setProject(event.target.value)}
              placeholder="Project ID"
              autoComplete="off"
              spellCheck={false}
              className={`${fieldClass} w-56`}
            />
            <Input
              aria-label="Google Cloud location"
              value={location}
              disabled={disabled}
              onChange={(event) => setLocation(event.target.value)}
              placeholder="Location"
              autoComplete="off"
              spellCheck={false}
              className={`${fieldClass} w-32`}
            />
          </div>
          <p className="text-[12px] text-ink-3">
            {payAsYouGo
              ? 'Usage is billed to this project.'
              : 'Only needed when your Gemini Enterprise license is not found from your account.'}
          </p>
        </>
      ) : null}
    </div>
  )
}
