import { useCallback, useEffect, useRef, useState } from 'react'
import type { BrowserPasswordAction, BrowserPasswordAPI, BrowserPasswordState } from '@shared/browserPasswords'

export type BrowserPasswordsController = {
  api: BrowserPasswordAPI | undefined
  state: BrowserPasswordState
  busy: boolean
  error: string
  act: (action: BrowserPasswordAction) => Promise<BrowserPasswordState | undefined>
}

export function useBrowserPasswords(webContentsId: number | null): BrowserPasswordsController {
  const api = window.jaz?.browserPasswords
  const [state, setState] = useState<BrowserPasswordState>({ origin: '', usernames: [], entries: [], locked: true })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const revision = useRef(0)

  useEffect(() => {
    if (!api || webContentsId === null) {
      return
    }
    let active = true
    const refresh = async () => {
      const request = ++revision.current
      try {
        const next = await api.state(webContentsId)
        if (active && request === revision.current) {
          setState(next)
          setError('')
        }
      } catch {
        if (active) {
          setError('Passwords could not be loaded.')
        }
      }
    }
    void refresh()
    const unsubscribe = api.subscribe(webContentsId, () => void refresh())
    return () => {
      active = false
      unsubscribe()
    }
  }, [api, webContentsId])

  const act = useCallback(async (action: BrowserPasswordAction) => {
    if (!api || webContentsId === null) {
      return undefined
    }
    const request = ++revision.current
    setBusy(true)
    setError('')
    try {
      const next = await api.act(webContentsId, action)
      if (request !== revision.current) {
        return undefined
      }
      setState(next)
      return next
    } catch {
      setError('The password action failed. Try again.')
      return undefined
    } finally {
      setBusy(false)
    }
  }, [api, webContentsId])

  return { api, state, busy, error, act }
}
