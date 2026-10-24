import { createContext } from 'react'

// On desktop a thread's side panel floats as its own card beside the content
// card: `slot` is where it mounts and `row` is the width the two share.
export const SidePanelSlot = createContext<{ slot: HTMLDivElement | null; row: HTMLDivElement | null }>({
  slot: null,
  row: null,
})
