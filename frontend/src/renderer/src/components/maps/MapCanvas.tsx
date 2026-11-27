import 'maplibre-gl/dist/maplibre-gl.css'
import './map.css'
import { LngLatBounds, Map as MapLibre, Marker, NavigationControl, setWorkerUrl, type MapOptions } from 'maplibre-gl'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import { useEffect, useRef, useState } from 'react'
import type { MapPlace } from '@/lib/maps'
import { useTheme } from '@/lib/theme'

// The worker's default location is resolved from import.meta.url, which is
// only http(s) in a dev server; the bundled URL works under file:// too.
setWorkerUrl(workerUrl)

const PIN = 'group/pin cursor-pointer p-1 outline-none'
const PIN_FACE =
  'grid size-6 place-items-center rounded-full bg-ink text-[11px] font-semibold tabular-nums text-bg shadow-raised ring-2 ring-bg transition-[scale,background-color,color] duration-150 ease-out group-hover/pin:scale-110 group-focus-visible/pin:ring-primary group-data-[active=true]/pin:scale-[1.2] group-data-[active=true]/pin:bg-primary group-data-[active=true]/pin:text-on-primary'

function styleURL(theme: 'light' | 'dark'): string {
  return `https://tiles.openfreemap.org/styles/${theme === 'dark' ? 'dark' : 'positron'}`
}

function camera(places: MapPlace[]): Pick<MapOptions, 'center' | 'zoom' | 'bounds' | 'fitBoundsOptions'> {
  if (places.length === 1) return { center: [places[0].longitude, places[0].latitude], zoom: 15 }
  const bounds = new LngLatBounds()
  for (const place of places) bounds.extend([place.longitude, place.latitude])
  return { bounds, fitBoundsOptions: { padding: 48, maxZoom: 15 } }
}

export default function MapCanvas({ places, selected, highlighted, onSelect }: {
  places: MapPlace[]
  selected?: number
  highlighted?: number
  onSelect: (index: number) => void
}) {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<MapLibre | null>(null)
  const pins = useRef<HTMLButtonElement[]>([])
  const select = useRef(onSelect)
  select.current = onSelect
  const { resolved } = useTheme()
  const style = useRef(styleURL(resolved))
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let instance: MapLibre
    try {
      instance = new MapLibre({
        container: container.current!,
        style: style.current,
        cooperativeGestures: true,
        dragRotate: false,
        pitchWithRotate: false,
        touchPitch: false,
        attributionControl: { compact: true },
        ...camera(places),
      })
    } catch {
      setFailed(true)
      return
    }
    const fail = () => setFailed(true)
    instance.on('error', fail)
    instance.once('load', () => instance.off('error', fail))
    instance.touchZoomRotate.disableRotation()
    instance.addControl(new NavigationControl({ showCompass: false }), 'top-right')
    pins.current = places.map((place, index) => {
      const pin = document.createElement('button')
      pin.type = 'button'
      pin.className = PIN
      pin.setAttribute('aria-label', `${index + 1}. ${place.name}`)
      const face = document.createElement('span')
      face.className = PIN_FACE
      face.textContent = String(index + 1)
      pin.append(face)
      pin.addEventListener('click', (event) => {
        event.stopPropagation()
        select.current(index)
      })
      new Marker({ element: pin }).setLngLat([place.longitude, place.latitude]).addTo(instance)
      return pin
    })
    map.current = instance
    return () => {
      instance.remove()
      map.current = null
    }
  }, [places])

  useEffect(() => {
    const next = styleURL(resolved)
    if (next === style.current) return
    style.current = next
    map.current?.setStyle(next)
  }, [resolved])

  useEffect(() => {
    // Earlier places stack above later ones, and the one in focus above all.
    pins.current.forEach((pin, index) => {
      pin.dataset.active = String(index === highlighted)
      pin.style.zIndex = String(index === highlighted ? places.length : places.length - index - 1)
    })
  }, [highlighted, places])

  useEffect(() => {
    const place = selected === undefined ? undefined : places[selected]
    const instance = map.current
    if (!place || !instance) return
    instance.easeTo({ center: [place.longitude, place.latitude], zoom: Math.max(instance.getZoom(), 14), duration: 450 })
  }, [selected, places])

  return (
    <>
      <div ref={container} className="jaz-map isolate h-full w-full" />
      {failed ? <p className="absolute inset-0 grid place-items-center bg-surface px-6 text-center text-[12.5px] text-ink-3">The map could not load. The places and their directions are below.</p> : null}
    </>
  )
}
