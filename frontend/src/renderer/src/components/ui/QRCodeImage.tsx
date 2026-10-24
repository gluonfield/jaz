import { LoaderCircle } from 'lucide-react'
import * as QRCode from 'qrcode'
import { useEffect, useState } from 'react'

export function QRCodeImage({ value, alt, className = 'size-full' }: { value: string; alt: string; className?: string }) {
  const [src, setSrc] = useState('')

  useEffect(() => {
    let cancelled = false
    setSrc('')
    void QRCode.toDataURL(value, {
      errorCorrectionLevel: 'M',
      margin: 1,
      width: 448,
      color: { dark: '#111111', light: '#ffffff' },
    })
      .then((next) => {
        if (!cancelled) setSrc(next)
      })
      .catch(() => {
        if (!cancelled) setSrc('')
      })
    return () => {
      cancelled = true
    }
  }, [value])

  return src ? <img src={src} alt={alt} className={className} /> : <LoaderCircle size={18} className="animate-spin text-ink-3" />
}
