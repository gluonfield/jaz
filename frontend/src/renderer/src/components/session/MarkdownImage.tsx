import { ImageOff } from 'lucide-react'
import { createContext, useContext, useState, type ComponentProps } from 'react'
import { Modal } from '@/components/ui/Modal'

export const MarkdownImageLinkContext = createContext(false)

export function MarkdownImage({ src, alt = '', title }: ComponentProps<'img'>) {
  const linked = useContext(MarkdownImageLinkContext)
  const [failedSource, setFailedSource] = useState('')
  const [open, setOpen] = useState(false)
  if (!src || src === failedSource) {
    return (
      <span className="inline-flex max-w-full items-center gap-2 rounded-[8px] bg-surface-2 px-3 py-2 text-[12px] text-ink-3" title={title}>
        <ImageOff size={16} className="shrink-0" aria-hidden />
        <span>{alt ? alt + ' — image unavailable' : 'Image unavailable'}</span>
      </span>
    )
  }
  const image = (
    <img
      src={src}
      alt={alt}
      title={title}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onError={() => setFailedSource(src)}
      className="block h-auto max-h-[32rem] max-w-full rounded-[8px] object-contain outline outline-1 -outline-offset-1 outline-black/10 dark:outline-white/10"
    />
  )
  if (linked) {
    return image
  }
  return (
    <>
      <button
        type="button"
        aria-label={alt ? 'Expand image: ' + alt : 'Expand image'}
        className="inline-block max-w-full cursor-zoom-in rounded-[8px] align-middle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
        onClick={() => setOpen(true)}
      >
        {image}
      </button>
      {open ? (
        <Modal open={open} onClose={() => setOpen(false)} title={alt || 'Image'} size="xl" chromeless>
          <div className="grid min-h-0 place-items-center bg-bg p-3 sm:p-4">
            <img
              src={src}
              alt={alt}
              referrerPolicy="no-referrer"
              className="max-h-[calc(100dvh-7rem)] max-w-full rounded-[8px] object-contain"
            />
          </div>
        </Modal>
      ) : null}
    </>
  )
}
