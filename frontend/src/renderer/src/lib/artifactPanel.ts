import { createContext } from 'react'
import type { ArtifactInput } from '@/lib/artifacts'
import type { PublishedAppRef } from '@/lib/sidePanelTabs'

// ArtifactPanel is the thread's side panel as an inline artifact sees it:
// where the artifact, or the app published from it, opens beside the chat.
export const ArtifactPanelContext = createContext<{
  openArtifact: (artifact: ArtifactInput) => void
  openPublishedApp: (app: PublishedAppRef) => void
} | null>(null)
