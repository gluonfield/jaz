import { terminateLocalBackend as terminateActual } from '@main/backend'

export let preparations = 0
let actual = false
export const pending: { resolve: () => void; reject: (error: Error) => void }[] = []

export function useActualBackend(): void {
  actual = true
}

export function terminateLocalBackend(options: { timeoutMs: number; forceTimeoutMs: number }, reason: string): Promise<void> {
  preparations += 1
  return actual ? terminateActual(options, reason) : new Promise((resolve, reject) => pending.push({ resolve, reject }))
}
