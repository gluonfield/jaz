import { useEffect, useRef } from 'react'

// What is polled while the agent works is read once more when its turn ends,
// for whatever it wrote after the last poll.
export function useRefetchAfterWork(working: boolean, refetch: () => unknown) {
  const wasWorking = useRef(working)
  useEffect(() => {
    if (wasWorking.current && !working) void refetch()
    wasWorking.current = working
  }, [working, refetch])
}
