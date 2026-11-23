import { ExternalLink, QrCode } from 'lucide-react'
import { useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { Popover } from '@/components/ui/Popover'
import { QRCodeImage } from '@/components/ui/QRCodeImage'
import { openExternal } from '@/lib/clientRuntime'
import { googlePasskeyDestination } from '@shared/preview'

export function BrowserAddressActions({ url, title, visible }: { url: string; title?: string; visible: boolean }) {
  const [open, setOpen] = useState(false)
  const shown = open && visible
  const googleDestination = googlePasskeyDestination(url, title)
  const destination = googleDestination ?? url
  const openInBrowser = () => openExternal(destination)
  return <div className="-mr-1.5 flex shrink-0 items-center opacity-0 transition-opacity duration-150 group-hover/address:opacity-100 has-focus-visible:opacity-100 has-aria-expanded:opacity-100 pointer-coarse:opacity-100">
    <Popover
      open={shown}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={<IconButton size="2xs" className="pointer-coarse:size-7" aria-label="Open on phone" title="Open on phone" aria-expanded={shown} onClick={() => setOpen((value) => !value)}><QrCode size={12} /></IconButton>}
    >
      <div className="grid size-41 place-items-center rounded-lg bg-white p-1.5">
        <QRCodeImage value={destination} label={googleDestination ? 'QR code to start a separate Google sign-in' : 'QR code for this page'} />
      </div>
    </Popover>
    <IconButton size="2xs" className="pointer-coarse:size-7" aria-label="Open in Browser" title="Open in Browser" onClick={openInBrowser}><ExternalLink size={12} /></IconButton>
  </div>
}
