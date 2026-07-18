import { Circle, CircleCheck, LoaderCircle } from 'lucide-react'
import type { ReactElement } from 'react'
import type { TaskStepState } from '@/lib/taskSurface'

export function TaskStepIcon({
  state,
  animate,
}: {
  state: TaskStepState
  animate: boolean
}): ReactElement {
  switch (state) {
    case 'completed':
      return <CircleCheck size={14} className="text-primary" strokeWidth={2.25} aria-hidden />
    case 'active':
      return (
        <LoaderCircle
          size={14}
          className={`text-running ${animate ? 'animate-spin' : ''}`}
          aria-hidden
        />
      )
    case 'pending':
      return <Circle size={14} className="text-ink-3" aria-hidden />
  }
}
