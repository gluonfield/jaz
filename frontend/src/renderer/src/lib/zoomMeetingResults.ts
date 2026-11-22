import { z } from 'zod'
import type { MCPAppEvent } from '@/lib/api/types'

const meetingList = z.looseObject({ meetings: z.array(z.unknown()) })
const displayText = z.string().catch('')
const participantLink = z.string().transform((value, ctx) => {
  const url = URL.parse(value)
  if (url && url.protocol === 'https:' && !url.username && !url.password && !url.port
    && /(^|\.)zoom\.(us|com)$/.test(url.hostname) && /^\/j\/\d+\/?$/.test(url.pathname)) {
    return url.href
  }
  ctx.addIssue({ code: 'custom', message: 'Expected a Zoom participant URL' })
  return z.NEVER
})
const upcomingMeeting = z.object({
  meeting_category: z.literal('scheduled_upcoming'),
  topic: displayText,
  schedule_start_time: displayText,
  schedule_end_time: displayText,
  host_name: displayText,
  join_url: participantLink,
})

type ZoomJoinableMeeting = z.infer<typeof upcomingMeeting>

export function zoomMeetingResults(app: MCPAppEvent) {
  const { result } = app
  if ((!app.server_id.startsWith('zoom:') && !app.server_id.startsWith('plugin:zoom:')) || app.tool !== 'search_meetings' || result.isError) return
  const list = meetingList.safeParse(result.structuredContent)
  if (!list.success) return

  const joinable: ZoomJoinableMeeting[] = []
  const otherMeetings: unknown[] = []
  for (const row of list.data.meetings) {
    const meeting = upcomingMeeting.safeParse(row)
    if (meeting.success) {
      joinable.push(meeting.data)
    } else {
      otherMeetings.push(row)
    }
  }
  if (!joinable.length) return

  // Zoom's widget reads structuredContent. Keep the original result for the transcript and model.
  const providerResult = otherMeetings.length ? {
    ...result,
    structuredContent: { ...list.data, meetings: otherMeetings },
  } : undefined
  return { joinable, providerResult }
}
