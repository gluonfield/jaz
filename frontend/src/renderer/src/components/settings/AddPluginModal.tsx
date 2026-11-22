import { type FormEvent, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Checkbox } from '@/components/ui/Checkbox'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import type { PluginInstallInput } from '@/lib/api/plugins'

export function AddPluginModal({
  open,
  busy,
  onClose,
  onInstall,
  onAddSource,
}: {
  open: boolean
  busy: boolean
  onClose: () => void
  onInstall: (input: PluginInstallInput) => void
  onAddSource: (input: PluginInstallInput) => void
}) {
  const [repository, setRepository] = useState('')
  const [ref, setRef] = useState('')
  const [path, setPath] = useState('')
  const [asMarketplace, setAsMarketplace] = useState(false)
  const [autoUpdate, setAutoUpdate] = useState(false)

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const repo = repository.trim()
    if (!repo || busy) return
    const input: PluginInstallInput = {
      repository: repo,
      ref: ref.trim() || undefined,
      paths: path.trim() ? [path.trim()] : undefined,
      auto_update: autoUpdate || undefined,
    }
    if (asMarketplace) {
      onAddSource(input)
      return
    }
    onInstall(input)
  }

  return (
    <Modal open={open} onClose={onClose} title="Add plugin from Git" description="Install a plugin repository directly or register a Git marketplace catalog.">
      <form className="flex flex-col gap-3.5" onSubmit={submit}>
        <label className="flex flex-col gap-1 text-[12px] text-ink-2">
          Repository
          <Input
            value={repository}
            onChange={(event) => setRepository(event.target.value)}
            placeholder="owner/repo or https://github.com/owner/repo"
            autoComplete="off"
            autoFocus
            disabled={busy}
          />
        </label>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-[12px] text-ink-2">
            Branch, tag or commit (optional)
            <Input
              value={ref}
              onChange={(event) => setRef(event.target.value)}
              placeholder="main"
              autoComplete="off"
              disabled={busy}
            />
          </label>
          <label className="flex flex-col gap-1 text-[12px] text-ink-2">
            Subpath (optional)
            <Input
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder="plugins/my-plugin"
              autoComplete="off"
              disabled={busy}
            />
          </label>
        </div>
        <div className="space-y-2 pt-1">
          <label className="flex cursor-pointer items-center gap-2.5 text-[13px] text-ink">
            <Checkbox checked={asMarketplace} onChange={setAsMarketplace} disabled={busy} />
            <span>Add as marketplace source (browse &amp; install plugins individually)</span>
          </label>
          <label className="flex cursor-pointer items-center gap-2.5 text-[13px] text-ink">
            <Checkbox checked={autoUpdate} onChange={setAutoUpdate} disabled={busy} />
            <span>Auto-update when new commits are pushed</span>
          </label>
        </div>
        <p className="text-[12px] leading-5 text-ink-3">
          Private repositories use Git credentials configured on the Jaz backend. Plugins may run MCP commands on the backend, so install only from sources you trust.
        </p>
        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !repository.trim()}>
            {busy ? (asMarketplace ? 'Adding…' : 'Installing…') : asMarketplace ? 'Add source' : 'Install'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
