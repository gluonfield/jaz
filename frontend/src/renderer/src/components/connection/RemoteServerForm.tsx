import { LoaderCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { DEFAULT_CLOUD_URL } from '@/lib/cloudSession'

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
  onSubmit: () => void
  onBack: () => void
  busy: boolean
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        onSubmit()
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
        placeholder={DEFAULT_CLOUD_URL}
        spellCheck={false}
        className="rounded-full font-mono text-[12px]"
      />
      <div className="flex items-center justify-end gap-2">
        <Button variant="ghost" disabled={busy} onClick={onBack}>
          Back
        </Button>
        <Button variant="primary" type="submit" disabled={busy || !value.trim()}>
          {busy ? <LoaderCircle size={14} className="animate-spin" /> : null}
          {busy ? 'Connecting…' : 'Connect'}
        </Button>
      </div>
    </form>
  )
}
