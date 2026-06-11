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
// empty list so the picker simply doesn't open. Skills change when the user
// edits ~/.jaz/skills — rarely mid-session, hence the long staleTime.
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
