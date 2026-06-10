import type { MusicPreviewCategory } from '@/lib/music/itunesPreview'

export const MUSIC_BUBBLE_CATEGORIES: MusicPreviewCategory[] = [
  {
    id: 'pop',
    label: 'pop',
    queries: ['bright pop hooks', 'indie pop new wave', 'dance pop'],
  },
  {
    id: 'rock',
    label: 'rock',
    queries: ['garage rock', 'classic rock riffs', 'alternative rock', 'Against All Evil'],
    trackIds: ['551771665'],
  },
  {
    id: 'metal',
    label: 'metal',
    queries: ['Bullet for My Valentine', 'Trivium', 'Metallica', 'metal'],
  },
  {
    id: 'anime',
    label: 'anime',
    queries: ['anime opening', 'anime soundtrack', 'j-pop anime'],
  },
  {
    id: 'classical',
    label: 'classical',
    queries: ['piano concerto', 'string quartet', 'classical morning'],
  },
  {
    id: 'lofi',
    label: 'lo-fi',
    queries: ['lofi beats', 'chillhop', 'study beats'],
  },
  {
    id: 'jazz',
    label: 'jazz',
    queries: ['Avishai Cohen', 'Avishai Cohen trio', 'jazz', 'modern jazz'],
  },
  {
    id: 'electronic',
    label: 'electronic',
    queries: ['ambient electronic', 'synthwave', 'downtempo electronic'],
  },
  {
    id: 'top-charts',
    label: 'top charts',
    chartFeeds: [
      {
        label: 'top charts',
        url: 'https://itunes.apple.com/us/rss/topsongs/limit=50/json',
      },
    ],
  },
  {
    id: 'most-played',
    label: 'most played',
    chartFeeds: [
      {
        label: 'most played',
        url: 'https://rss.applemarketingtools.com/api/v2/us/music/most-played/50/songs.json',
      },
    ],
  },
]
