// A map of places, as the show_map tool presents it and a `jaz-map` Markdown
// fence embeds it. The backend owns geocoding, so every place has coordinates.
export interface MapPlace {
  name: string
  latitude: number
  longitude: number
  address?: string
  category?: string
  note?: string
  rating?: number
  reviews?: number
  price?: string
  url?: string
}

export interface PlaceMap {
  title: string
  places: MapPlace[]
}

const text = (value: unknown): string | undefined => (typeof value === 'string' && value.trim()) || undefined
const finite = (value: unknown): number | undefined => (typeof value === 'number' && Number.isFinite(value) ? value : undefined)

function webURL(value: unknown): string | undefined {
  const url = text(value)
  if (!url || !URL.canParse(url)) return undefined
  const { protocol } = new URL(url)
  return protocol === 'https:' || protocol === 'http:' ? url : undefined
}

function place(value: unknown): MapPlace[] {
  if (typeof value !== 'object' || value === null) return []
  const raw = value as Record<string, unknown>
  const name = text(raw.name)
  const latitude = finite(raw.latitude)
  const longitude = finite(raw.longitude)
  if (!name || latitude === undefined || longitude === undefined || Math.abs(latitude) > 90 || Math.abs(longitude) > 180) return []
  const rating = finite(raw.rating)
  const reviews = finite(raw.reviews)
  return [{
    name,
    latitude,
    longitude,
    address: text(raw.address),
    category: text(raw.category),
    note: text(raw.note),
    rating: rating !== undefined && rating > 0 && rating <= 5 ? rating : undefined,
    reviews: reviews !== undefined && reviews > 0 ? Math.round(reviews) : undefined,
    price: text(raw.price),
    url: webURL(raw.url),
  }]
}

// Reads a map from a stored presentation or a fence's JSON, keeping only the
// places it can pin and the links it can safely open.
export function parsePlaceMap(value: unknown): PlaceMap | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const raw = value as Record<string, unknown>
  const places = Array.isArray(raw.places) ? raw.places.flatMap(place) : []
  return places.length ? { title: text(raw.title) ?? 'Map', places } : undefined
}

export function parseMapFence(code: string): PlaceMap | undefined {
  try {
    return parsePlaceMap(JSON.parse(code))
  } catch {
    return undefined
  }
}

// A maps app resolves "name, address" to the business listing; without an
// address the coordinates are the only unambiguous destination.
function listing(place: MapPlace): string | undefined {
  return place.address ? `${place.name}, ${place.address}` : undefined
}

function point(place: MapPlace): string {
  return `${place.latitude},${place.longitude}`
}

export function googleMapsURL(place: MapPlace): string {
  return `https://www.google.com/maps/search/?${new URLSearchParams({ api: '1', query: listing(place) ?? point(place) })}`
}

// Apple searches for the listing near the pin, or drops a pin named for the place.
export function appleMapsURL(place: MapPlace): string {
  const query = listing(place)
  return `https://maps.apple.com/?${new URLSearchParams(query ? { q: query, sll: point(place) } : { q: place.name, ll: point(place) })}`
}
