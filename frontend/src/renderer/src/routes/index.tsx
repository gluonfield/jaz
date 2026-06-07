import { createFileRoute, redirect } from '@tanstack/react-router'

// The new-thread page is the app's landing surface.
export const Route = createFileRoute('/')({
  beforeLoad: () => {
    throw redirect({ to: '/new' })
  },
})
