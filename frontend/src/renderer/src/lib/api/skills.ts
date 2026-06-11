import { queryOptions } from '@tanstack/react-query'
import { keys } from '../query/keys'
import { get } from './client'

export interface SkillInfo {
  name: string
  description: string
  path: string
}

// The skill catalog for the composer's $-mention picker. Resilient by design:
// any failure (older backend without the route, no skills installed) yields an
// empty list so the picker simply doesn't open. The composer refetches when a
// new $ trigger opens, so edits to ~/.jaz/skills are picked up mid-session.
export const skillsQuery = queryOptions({
  queryKey: keys.skills,
  queryFn: async () => {
    try {
      const data = await get<{ skills: SkillInfo[] | null }>('/v1/skills')
      return data.skills ?? []
    } catch {
      return []
    }
  },
  staleTime: 5 * 60_000,
})
