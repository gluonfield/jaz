import { apiFetch } from '@/lib/api/client'

export type MusicPreviewCategory = {
  id: string
  label: string
  queries?: string[]
  chartFeeds?: ChartFeed[]
  trackIds?: string[]
}

export type ChartFeed = {
  label: string
  url: string
}

export type PreviewTrack = {
  id: string
  artistName: string
  trackName: string
  collectionName?: string
  previewUrl: string
  trackViewUrl?: string
  artworkUrl?: string
  query: string
}

type ITunesSearchResponse = {
  resultCount?: number
  results?: ITunesSearchResult[]
}

type ITunesSearchResult = {
  wrapperType?: string
  kind?: string
  trackId?: number
  artistName?: string
  trackName?: string
  collectionName?: string
  previewUrl?: string
  trackViewUrl?: string
  artworkUrl100?: string
}

const searchCache = new Map<string, Promise<PreviewTrack[]>>()
const chartCache = new Map<string, Promise<string[]>>()

function randomItem<T>(items: T[]): T {
  return items[Math.floor(Math.random() * items.length)]
}

type LookupResponse = {
  results?: ITunesSearchResult[]
}

type RSSTopSongsFeed = {
  feed?: {
    entry?: Array<{
      id?: {
        attributes?: {
          'im:id'?: string
        }
      }
    }>
  }
}

type MarketingToolsFeed = {
  feed?: {
    results?: Array<{
      id?: string
    }>
  }
}

function shuffle<T>(items: T[]): T[] {
  const next = [...items]
  for (let i = next.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    const item = next[i]
    next[i] = next[j]
    next[j] = item
  }
  return next
}

function normalizeTrack(result: ITunesSearchResult, query: string): PreviewTrack | null {
  if (result.wrapperType !== 'track' || result.kind !== 'song') return null
  if (!result.previewUrl || !result.artistName || !result.trackName) return null
  return {
    id: String(result.trackId ?? result.previewUrl),
    artistName: result.artistName,
    trackName: result.trackName,
    collectionName: result.collectionName,
    previewUrl: result.previewUrl,
    trackViewUrl: result.trackViewUrl,
    artworkUrl: result.artworkUrl100,
    query,
  }
}

export function searchPreviewTracks(query: string, country = 'US'): Promise<PreviewTrack[]> {
  const trimmed = query.trim()
  if (!trimmed) return Promise.resolve([])

  const cacheKey = `${country}:${trimmed.toLowerCase()}`
  const cached = searchCache.get(cacheKey)
  if (cached) return cached

  const request = (async () => {
    const url = new URL('https://itunes.apple.com/search')
    url.searchParams.set('term', trimmed)
    url.searchParams.set('media', 'music')
    url.searchParams.set('entity', 'song')
    url.searchParams.set('country', country)
    url.searchParams.set('limit', '50')
    url.searchParams.set('explicit', 'No')

    const response = await fetch(url)
    if (!response.ok) throw new Error(`Preview search failed with ${response.status}`)

    const body = (await response.json()) as ITunesSearchResponse
    return (body.results ?? [])
      .map((result) => normalizeTrack(result, trimmed))
      .filter((track): track is PreviewTrack => track !== null)
  })()

  searchCache.set(cacheKey, request)
  return request
}

async function lookupPreviewTracks(ids: string[], source: string, country = 'US'): Promise<PreviewTrack[]> {
  const uniqueIds = Array.from(new Set(ids.map((id) => id.trim()).filter(Boolean))).slice(0, 50)
  if (uniqueIds.length === 0) return []

  const url = new URL('https://itunes.apple.com/lookup')
  url.searchParams.set('id', uniqueIds.join(','))
  url.searchParams.set('entity', 'song')
  url.searchParams.set('country', country)
  url.searchParams.set('explicit', 'No')

  const response = await fetch(url)
  if (!response.ok) throw new Error(`Preview lookup failed with ${response.status}`)

  const body = (await response.json()) as LookupResponse
  return (body.results ?? [])
    .map((result) => normalizeTrack(result, source))
    .filter((track): track is PreviewTrack => track !== null)
}

function idsFromChartFeed(feed: RSSTopSongsFeed | MarketingToolsFeed): string[] {
  const marketingIds = (feed as MarketingToolsFeed).feed?.results?.map((result) => result.id ?? '') ?? []
  if (marketingIds.length > 0) return marketingIds
  return (
    (feed as RSSTopSongsFeed).feed?.entry?.map(
      (entry) => entry.id?.attributes?.['im:id'] ?? '',
    ) ?? []
  )
}

async function chartTrackIds(feed: ChartFeed): Promise<string[]> {
  const cached = chartCache.get(feed.url)
  if (cached) return cached

  const request = (async () => {
    const body = await fetchChartFeed(feed)
    return idsFromChartFeed(body).filter(Boolean)
  })()

  chartCache.set(feed.url, request)
  return request
}

async function fetchChartFeed(feed: ChartFeed): Promise<RSSTopSongsFeed | MarketingToolsFeed> {
  try {
    const response = await fetch(feed.url)
    if (!response.ok) throw new Error(`${feed.label} failed with ${response.status}`)
    return (await response.json()) as RSSTopSongsFeed | MarketingToolsFeed
  } catch {
    const response = await apiFetch(`/v1/music/chart-feed?url=${encodeURIComponent(feed.url)}`)
    if (!response.ok) throw new Error(`${feed.label} failed with ${response.status}`)
    return (await response.json()) as RSSTopSongsFeed | MarketingToolsFeed
  }
}

async function chartPreviewTracks(feed: ChartFeed): Promise<PreviewTrack[]> {
  const ids = await chartTrackIds(feed)
  return lookupPreviewTracks(shuffle(ids).slice(0, 20), feed.label)
}

export async function pickRandomPreviewTrack(
  category: MusicPreviewCategory,
  excludeTrackId?: string,
): Promise<PreviewTrack> {
  const sources: Array<{ label: string; load: () => Promise<PreviewTrack[]> }> = [
    ...(category.trackIds?.length
      ? [
          {
            label: category.label,
            load: () => lookupPreviewTracks(category.trackIds ?? [], category.label),
          },
        ]
      : []),
    ...(category.chartFeeds ?? []).map((feed) => ({
      label: feed.label,
      load: () => chartPreviewTracks(feed),
    })),
    ...(category.queries ?? [])
      .map((query) => query.trim())
      .filter(Boolean)
      .map((query) => ({
        label: query,
        load: () => searchPreviewTracks(query),
      })),
  ]

  if (sources.length === 0) throw new Error(`No preview sources configured for ${category.label}`)

  const misses: string[] = []
  for (const source of shuffle(sources)) {
    try {
      const tracks = await source.load()
      // avoid replaying the track that just ended unless it's all we have
      const fresh = excludeTrackId ? tracks.filter((track) => track.id !== excludeTrackId) : tracks
      if (fresh.length > 0) return randomItem(fresh)
      if (tracks.length > 0) return randomItem(tracks)
    } catch {
      // Try the next configured source before surfacing an error.
    }
    misses.push(source.label)
  }

  throw new Error(`No previews found for ${misses.join(', ')}`)
}
