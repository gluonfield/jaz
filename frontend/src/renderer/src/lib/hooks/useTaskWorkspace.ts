import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { listFilesystemDirs } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'

export function useTaskWorkspace(directory: string) {
  const [choice, setChoice] = useState(true)
  const source = useQuery({
    queryKey: keys.filesystemDirs(directory),
    queryFn: () => listFilesystemDirs(directory),
    enabled: directory !== '',
    staleTime: 30_000,
    retry: false,
  })
  return {
    choice,
    setChoice,
    source,
    worktree: source.data?.git === true && choice,
    ready: !directory || !choice || source.isSuccess,
  }
}
