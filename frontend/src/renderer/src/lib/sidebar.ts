import { createContext } from 'react'

export const SidebarVisibility = createContext<((open: boolean) => void) | null>(null)
