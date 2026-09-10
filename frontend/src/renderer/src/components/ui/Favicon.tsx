import { Globe } from 'lucide-react'
import { memo, useState } from 'react'

export const Favicon = memo(function Favicon({
  url,
  className = 'size-3.5 shrink-0 text-ink-3',
}: {
  url: string
  className?: string
}) {
  const [failedDomain, setFailedDomain] = useState('')
  let domain: string
  try {
    domain = new URL(url).hostname
  } catch {
    domain = ''
  }
  if (!domain || domain === failedDomain) return <Globe size={14} className={className} aria-hidden />
  return (
    <img
      src={`https://www.google.com/s2/favicons?domain=${encodeURIComponent(domain)}&sz=64`}
      alt=""
      width={14}
      height={14}
      loading="lazy"
      onError={() => setFailedDomain(domain)}
      className={`${className} rounded-sm outline outline-1 outline-black/10 dark:outline-white/10`}
    />
  )
})
