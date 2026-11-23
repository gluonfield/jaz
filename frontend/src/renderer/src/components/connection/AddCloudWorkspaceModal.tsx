import { LoaderCircle } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { clientRuntime } from '@/lib/clientRuntime'
import { attachCloudSignIn, type CloudWorkspaceSummary } from '@/lib/cloudAccount'
import { DEFAULT_CLOUD_URL, useCloudSession } from '@/lib/cloudSession'

export function AddCloudWorkspaceModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add Cloud Workspace"
      description="Sign in to a Jaz Cloud server. Each workspace keeps its own threads and settings."
      size="sm"
    >
      <AddCloudWorkspaceForm onClose={onClose} />
    </Modal>
  )
}

function connectionKey(workspaces: CloudWorkspaceSummary[]): string {
  return workspaces.map((workspace) => `${workspace.id}@${workspace.connected_at ?? ''}`).join(' ')
}

// Mounted only while the modal is open, so every open starts from a clean form.
function AddCloudWorkspaceForm({ onClose }: { onClose: () => void }) {
  const cloud = useCloudSession()
  const [url, setUrl] = useState(() => {
    const candidate = clientRuntime.cloudUrl || DEFAULT_CLOUD_URL
    return cloud.workspaces.some((workspace) => workspace.url === candidate) ? '' : candidate
  })
  const [starting, setStarting] = useState(false)
  const [awaitingBrowser, setAwaitingBrowser] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)

  const connections = connectionKey(cloud.workspaces)
  const connectionsAtOpen = useRef(connections)
  useEffect(() => {
    if (connections !== connectionsAtOpen.current) onClose()
  }, [connections, onClose])

  const signIn = async () => {
    setStarting(true)
    setStartError(null)
    try {
      const err = await attachCloudSignIn(url.trim())
      if (err) {
        setStartError(err)
        setAwaitingBrowser(false)
        return
      }
      setAwaitingBrowser(clientRuntime.kind === 'electron')
    } catch (error) {
      setStartError(error instanceof Error ? error.message : 'Could not start sign-in')
    } finally {
      setStarting(false)
    }
  }

  const error = startError ?? (awaitingBrowser ? cloud.error : null)

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        void signIn()
      }}
      className="flex flex-col gap-4"
    >
      <div className="flex flex-col gap-2">
        <Input
          value={url}
          onChange={(event) => {
            setUrl(event.target.value)
            setStartError(null)
          }}
          placeholder={DEFAULT_CLOUD_URL}
          aria-label="Server URL"
          aria-invalid={error ? true : undefined}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          inputMode="url"
          disabled={starting}
          className="font-mono text-[12.5px]"
        />
        {error ? (
          <p role="alert" className="px-0.5 text-[12px] text-danger">
            {error}
          </p>
        ) : awaitingBrowser ? (
          <p role="status" className="flex items-center gap-1.5 px-0.5 text-[12px] text-ink-3">
            <LoaderCircle size={12} className="shrink-0 animate-spin" />
            Finish signing in in your browser.
          </p>
        ) : null}
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={starting || !url.trim()}>
          {starting ? <LoaderCircle size={14} className="animate-spin" /> : null}
          Sign in
        </Button>
      </div>
    </form>
  )
}
