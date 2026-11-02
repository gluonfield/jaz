import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { UpdatePanel } from '@/components/update/UpdatePanel'
import '@/styles/globals.css'

function Fixture() {
  const [clicks, setClicks] = useState(0)
  return (
    <main className="w-[300px] bg-surface p-3 text-ink">
      <UpdatePanel />
      <button id="interaction" onClick={() => setClicks((value) => value + 1)}>Interactions: {clicks}</button>
      <input aria-label="Draft" />
    </main>
  )
}

createRoot(document.getElementById('root')!).render(<Fixture />)
