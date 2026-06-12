import { useEffect, useRef } from 'react'

export function useWindowEvent<K extends keyof WindowEventMap>(
  type: K,
  handler: (event: WindowEventMap[K]) => void,
  enabled = true,
  options?: boolean | AddEventListenerOptions,
) {
  const handlerRef = useRef(handler)

  useEffect(() => {
    handlerRef.current = handler
  }, [handler])

  useEffect(() => {
    if (!enabled) return
    const listener = (event: WindowEventMap[K]) => handlerRef.current(event)
    window.addEventListener(type, listener as EventListener, options)
    return () => window.removeEventListener(type, listener as EventListener, options)
  }, [enabled, options, type])
}
