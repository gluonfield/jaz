import { ExternalLink, Globe, Navigation, Star } from 'lucide-react'
import { lazy, memo, Suspense, useCallback, useRef, useState, type MouseEvent } from 'react'
import { openExternal } from '@/lib/clientRuntime'
import { directionsURL, googleMapsURL, type MapPlace, type PlaceMap } from '@/lib/maps'

const MapCanvas = lazy(() => import('./MapCanvas'))

const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })
const ACTION = 'inline-flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium transition-[background-color,color,scale] duration-150 active:scale-[0.97]'

function open(event: MouseEvent<HTMLAnchorElement>) {
  event.preventDefault()
  event.stopPropagation()
  openExternal(event.currentTarget.href)
}

export const MapBlock = memo(function MapBlock({ map }: { map: PlaceMap }) {
  const [selected, setSelected] = useState<number>()
  const [hovered, setHovered] = useState<number>()
  const strip = useRef<HTMLDivElement>(null)
  // A pin brings its card into view by scrolling the strip alone, never the thread.
  const reveal = useCallback((index: number) => {
    setSelected(index)
    const list = strip.current
    const card = list?.children[index] as HTMLElement | undefined
    if (!list || !card) return
    const left = card.offsetLeft
    if (left < list.scrollLeft || left + card.offsetWidth > list.scrollLeft + list.clientWidth) {
      list.scrollTo({ left: left - 8, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
    }
  }, [])
  const single = map.places.length === 1
  return (
    <figure className="w-full">
      <figcaption className="flex h-8 items-center gap-2 text-[12.5px] text-ink-3">
        <span className="min-w-0 flex-1 truncate">{map.title}</span>
        {single ? null : <span className="shrink-0 tabular-nums">{map.places.length} places</span>}
      </figcaption>
      <div className="relative h-[280px] overflow-hidden rounded-card border border-border bg-surface">
        <Suspense fallback={null}>
          <MapCanvas places={map.places} selected={selected} highlighted={hovered ?? selected} onSelect={reveal} />
        </Suspense>
      </div>
      <div ref={strip} className="scrollbar-quiet relative -mx-1 mt-2 flex snap-x snap-mandatory scroll-px-1 gap-2 overflow-x-auto px-1 pb-1">
        {map.places.map((place, index) => (
          <PlaceCard
            key={index}
            place={place}
            index={index}
            single={single}
            active={index === selected}
            onSelect={setSelected}
            onHover={setHovered}
          />
        ))}
      </div>
    </figure>
  )
})

function PlaceCard({ place, index, single, active, onSelect, onHover }: {
  place: MapPlace
  index: number
  single: boolean
  active: boolean
  onSelect: (index: number) => void
  onHover: (index: number | undefined) => void
}) {
  const facts = [place.category, place.price].filter(Boolean).join(' · ')
  return (
    <article
      onClick={() => onSelect(index)}
      onMouseEnter={() => onHover(index)}
      onMouseLeave={() => onHover(undefined)}
      className={`flex shrink-0 snap-start cursor-pointer flex-col rounded-card border bg-bg p-3 transition-colors duration-150 ${single ? 'w-full' : 'w-[272px]'} ${active ? 'border-primary/55' : 'border-border hover:border-ink/20'}`}
    >
      <div className="flex items-start gap-2.5">
        <span className={`mt-px grid size-5 shrink-0 place-items-center rounded-full text-[10.5px] font-semibold tabular-nums transition-colors duration-150 ${active ? 'bg-primary text-on-primary' : 'bg-ink text-bg'}`} aria-hidden>
          {index + 1}
        </span>
        <div className="min-w-0 flex-1">
          <button type="button" className="block max-w-full cursor-pointer truncate text-left text-[13.5px] font-semibold leading-5 text-ink outline-none focus-visible:underline" onClick={(event) => { event.stopPropagation(); onSelect(index) }}>
            {place.name}
          </button>
          {facts || place.rating ? (
            <div className="flex items-center gap-1 truncate text-[12px] leading-5 text-ink-2">
              {place.rating ? (
                <span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums text-ink">
                  <Star size={11} className="fill-accent text-accent" aria-hidden />
                  {place.rating.toFixed(1)}
                  {place.reviews ? <span className="text-ink-3">({compact.format(place.reviews)})</span> : null}
                </span>
              ) : null}
              {place.rating && facts ? <span className="text-ink-3" aria-hidden>·</span> : null}
              {facts ? <span className="truncate">{facts}</span> : null}
            </div>
          ) : null}
        </div>
        {place.url ? (
          <a href={place.url} onClick={open} aria-label={`${place.name} website`} title="Website" className="-mr-1 -mt-1 grid size-7 shrink-0 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink">
            <Globe size={14} aria-hidden />
          </a>
        ) : null}
      </div>
      {place.address ? <div className="mt-1.5 line-clamp-2 pl-[30px] text-[12px] leading-[17px] text-ink-3">{place.address}</div> : null}
      {place.note ? <div className="mt-1 line-clamp-2 pl-[30px] text-[12.5px] leading-[18px] text-ink-2">{place.note}</div> : null}
      <div className="mt-auto flex items-center gap-1 pt-2.5 pl-[26px]">
        <a href={directionsURL(place)} onClick={open} className={`${ACTION} bg-surface-2 text-ink hover:bg-ink/10`}>
          <Navigation size={13} aria-hidden />
          Directions
        </a>
        <a href={googleMapsURL(place)} onClick={open} className={`${ACTION} text-ink-2 hover:bg-surface-2 hover:text-ink`}>
          <ExternalLink size={13} aria-hidden />
          Google Maps
        </a>
      </div>
    </article>
  )
}
