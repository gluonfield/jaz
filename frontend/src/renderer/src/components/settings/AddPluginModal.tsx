import { type FormEvent, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Checkbox } from '@/components/ui/Checkbox'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Segmented } from '@/components/ui/Segmented'
import type { PluginInstallInput } from '@/lib/api/plugins'

export type AddPluginMode = 'install' | 'marketplace'

export function AddPluginModal({
  open,
  initialMode = 'install',
  busy,
  onClose,
  onInstall,
  onAddSource,
}: {
  open: boolean
  initialMode?: AddPluginMode
  busy: boolean
  onClose: () => void
  onInstall: (input: PluginInstallInput) => void
  onAddSource: (input: PluginInstallInput) => void
}) {
  const [mode, setMode] = useState<AddPluginMode>(initialMode)
  const [repository, setRepository] = useState('')
  const [ref, setRef] = useState('')
  const [path, setPath] = useState('')
  const [autoUpdate, setAutoUpdate] = useState(initialMode === 'marketplace')

  const isMarketplace = mode === 'marketplace'

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const repo = repository.trim()
    if (!repo || busy) return
    const input: PluginInstallInput = {
      repository: repo,
      ref: ref.trim() || undefined,
      paths: !isMarketplace && path.trim() ? [path.trim()] : undefined,
      auto_update: autoUpdate || undefined,
    }
    if (isMarketplace) {
      onAddSource(input)
      return
    }
    onInstall(input)
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isMarketplace ? 'Add Git marketplace' : 'Add plugin from Git'}
      description={
        isMarketplace
          ? 'Register a Git repository with a marketplace catalog so its plugins appear in Jaz.'
          : 'Install a plugin repository directly into Jaz.'
      }
    >
      <form className="flex flex-col gap-3.5" onSubmit={submit}>
        <Segmented
          layoutId="add-plugin-mode"
          value={mode}
          onChange={(next) => {
            setMode(next)
            if (next === 'marketplace') setAutoUpdate(true)
          }}
          options={[
            { value: 'install', label: 'Install plugin' },
            { value: 'marketplace', label: 'Add marketplace' },
          ]}
        />

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

        <div className={isMarketplace ? 'grid grid-cols-1 gap-3' : 'grid grid-cols-1 gap-3 sm:grid-cols-2'}>
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
          {!isMarketplace ? (
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
          ) : null}
        </div>

        <label className="flex cursor-pointer items-center gap-2.5 pt-1 text-[13px] text-ink">
          <Checkbox checked={autoUpdate} onChange={setAutoUpdate} disabled={busy} />
          <span>Auto-update when new commits are pushed</span>
        </label>

        <p className="text-[12px] leading-5 text-ink-3">
          Private repositories use Git credentials configured on the Jaz backend. Install only from sources you trust.
        </p>

        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !repository.trim()}>
            {busy ? (isMarketplace ? 'Adding…' : 'Installing…') : isMarketplace ? 'Add marketplace' : 'Install'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
