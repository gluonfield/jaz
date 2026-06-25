import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'

type TitlebarEntry = {
  owner: symbol
  node: ReactNode
} | null

type TitlebarContextValue = {
  slot: ReactNode
  actions: ReactNode
  setSlot: (owner: symbol, node: ReactNode) => void
  clearSlot: (owner: symbol) => void
  setActions: (owner: symbol, node: ReactNode) => void
  clearActions: (owner: symbol) => void
}

const TitlebarContext = createContext<TitlebarContextValue | null>(null)

export function TitlebarProvider({ children }: { children: ReactNode }) {
  const [slot, setSlotEntry] = useState<TitlebarEntry>(null)
  const [actions, setActionsEntry] = useState<TitlebarEntry>(null)

  const setSlot = useCallback((owner: symbol, node: ReactNode) => {
    setSlotEntry({ owner, node })
  }, [])
  const clearSlot = useCallback((owner: symbol) => {
    setSlotEntry((entry) => (entry?.owner === owner ? null : entry))
  }, [])
  const setActions = useCallback((owner: symbol, node: ReactNode) => {
    setActionsEntry({ owner, node })
  }, [])
  const clearActions = useCallback((owner: symbol) => {
    setActionsEntry((entry) => (entry?.owner === owner ? null : entry))
  }, [])

  const value = useMemo(
    () => ({
      slot: slot?.node ?? null,
      actions: actions?.node ?? null,
      setSlot,
      clearSlot,
      setActions,
      clearActions,
    }),
    [actions, clearActions, clearSlot, setActions, setSlot, slot],
  )

  return <TitlebarContext.Provider value={value}>{children}</TitlebarContext.Provider>
}

export function TitlebarSlotOutlet() {
  return <>{useTitlebarContext().slot}</>
}

export function TitlebarActionsOutlet() {
  return <>{useTitlebarContext().actions}</>
}

export function useTitlebarSlot(node: ReactNode) {
  const { setSlot, clearSlot } = useTitlebarContext()
  const owner = useStableOwner()

  useLayoutEffect(() => {
    setSlot(owner, node)
    return () => clearSlot(owner)
  }, [clearSlot, node, owner, setSlot])
}

export function useTitlebarActions(node: ReactNode) {
  const { setActions, clearActions } = useTitlebarContext()
  const owner = useStableOwner()

  useLayoutEffect(() => {
    setActions(owner, node)
    return () => clearActions(owner)
  }, [clearActions, node, owner, setActions])
}

function useStableOwner() {
  const owner = useRef<symbol | null>(null)
  if (!owner.current) owner.current = Symbol()
  return owner.current
}

function useTitlebarContext() {
  const context = useContext(TitlebarContext)
  if (!context) throw new Error('titlebar slot used outside TitlebarProvider')
  return context
}
