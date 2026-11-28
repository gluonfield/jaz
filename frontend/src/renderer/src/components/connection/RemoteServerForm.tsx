import { LoaderCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

// The launch screen's Jaz Cloud address form. The caller owns the surrounding
// surface and the connect action; this owns only the field, the copy, and the
// connect/back controls.
export function RemoteServerForm({
  value,
  onChange,
  onSubmit,
  onBack,
  busy,
}: {
  value: string
  onChange: (value: string) => void
  onSubmit: (url: string) => void
  onBack: () => void
  busy: boolean
}) {
  const url = value.trim()
  const canConnect = !busy && Boolean(url)
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (canConnect) onSubmit(url)
      }}
      className="flex w-full flex-col gap-2.5"
    >
      <div className="px-0.5">
        <p className="text-[13px] font-medium text-ink">Connect to Jaz Cloud</p>
        <p className="mt-0.5 text-[12px] text-ink-3">Enter your Jaz Cloud server’s HTTPS address.</p>
      </div>
      <Input
        autoFocus
        value={value}
        onChange={(event) => onChange(event.target.value)}
        aria-label="Server URL"
        required
        spellCheck={false}
        className="rounded-full font-mono text-[12px]"
      />
      <div className="flex items-center justify-end gap-2">
        <Button variant="ghost" disabled={busy} onClick={onBack}>
          Back
        </Button>
        <Button variant="primary" type="submit" disabled={!canConnect}>
          {busy ? <LoaderCircle size={14} className="animate-spin" /> : null}
          {busy ? 'Connecting…' : 'Connect'}
        </Button>
      </div>
    </form>
  )
}
