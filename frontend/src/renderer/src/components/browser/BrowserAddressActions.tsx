import { ExternalLink, QrCode } from 'lucide-react'
import { useState } from 'react'
import { Popover } from '@/components/ui/Popover'
import { QRCodeImage } from '@/components/ui/QRCodeImage'
import { openExternal } from '@/lib/clientRuntime'

const action = 'grid size-5 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink pointer-coarse:size-7'

export function BrowserAddressActions({ url, visible }: { url: string; visible: boolean }) {
  const [open, setOpen] = useState(false)
  return <div className="-mr-1.5 flex shrink-0 items-center opacity-0 transition-opacity duration-150 group-hover/address:opacity-100 has-focus-visible:opacity-100 has-aria-expanded:opacity-100 pointer-coarse:opacity-100">
    <Popover
      open={open && visible}
      onClose={() => setOpen(false)}
      placement="below"
      align="end"
      trigger={<button type="button" aria-label="Open on phone" title="Open on phone" aria-expanded={open && visible} onClick={() => setOpen((value) => !value)} className={action}><QrCode size={12} /></button>}
    >
      <div className="grid size-41 place-items-center rounded-lg bg-white p-1.5">
        <QRCodeImage value={url} alt="QR code for this page" />
      </div>
    </Popover>
    <button type="button" aria-label="Open in Browser" title="Open in Browser" onClick={() => openExternal(url)} className={action}><ExternalLink size={12} /></button>
  </div>
}
