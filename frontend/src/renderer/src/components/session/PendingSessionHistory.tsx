import { Bubble } from '@/components/session/Bubble'
import { THREAD_COLUMN_CLASS } from '@/components/session/threadLayout'
import { Skeleton, SkeletonRows } from '@/components/ui/Skeleton'
import {
  materializeOptimisticUserMessage,
  type OptimisticUserMessage,
} from '@/lib/optimisticUserMessage'

export function PendingSessionHistory({
  sessionId,
  initialPrompt,
}: {
  sessionId: string
  initialPrompt?: OptimisticUserMessage
}) {
  return (
    <div className={`${THREAD_COLUMN_CLASS} pt-2`}>
      {initialPrompt ? (
        <Bubble
          message={materializeOptimisticUserMessage(initialPrompt)}
          attachmentSessionId={sessionId}
        />
      ) : (
        <>
          <Skeleton className="mb-6 h-7 w-64" />
          <SkeletonRows count={5} />
        </>
      )}
    </div>
  )
}
