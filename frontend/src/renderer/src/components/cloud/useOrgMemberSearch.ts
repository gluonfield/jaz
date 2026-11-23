import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { orgMembersQuery, type OrgMember } from '@/lib/api/cloudMembers'

export function useOrgMemberSearch(query: string, enabled: boolean) {
  const [debouncedQuery, setDebouncedQuery] = useState('')

  useEffect(() => {
    const trimmed = query.trim()
    if (!trimmed) {
      setDebouncedQuery('')
      return
    }
    const timer = window.setTimeout(() => setDebouncedQuery(trimmed), 150)
    return () => window.clearTimeout(timer)
  }, [query])

  const base = useQuery({ ...orgMembersQuery('', 50), enabled })
  const searched = useQuery({
    ...orgMembersQuery(debouncedQuery, 20),
    enabled: enabled && debouncedQuery.length > 0,
  })

  const members = useMemo(() => {
    const byId = new Map<string, OrgMember>()
    for (const member of base.data ?? []) byId.set(member.id, member)
    for (const member of searched.data ?? []) byId.set(member.id, member)
    return Array.from(byId.values())
  }, [base.data, searched.data])

  return {
    members,
    isSearching: base.isPending || searched.isFetching,
  }
}
