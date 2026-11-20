import { z } from 'zod'
import type { MCPAppEvent } from '@/lib/api/types'

const meetingList = z.looseObject({ meetings: z.array(z.unknown()) })
const upcomingMeeting = z.object({
  meeting_category: z.literal('scheduled_upcoming'),
  topic: z.string(),
  schedule_start_time: z.string().optional(),
  schedule_end_time: z.string().optional(),
  host_name: z.string().optional(),
  join_url: z.string(),
})

type ZoomJoinableMeeting = z.infer<typeof upcomingMeeting>

function participantLink(value: string): URL | undefined {
  const url = URL.parse(value)
  if (!url || url.protocol !== 'https:' || url.username || url.password || url.port) return
  const zoomHost = /(^|\.)zoom\.(us|com)$/.test(url.hostname)
  if (zoomHost && /^\/j\/\d+\/?$/.test(url.pathname)) return url
}

export function zoomMeetingResults(app: MCPAppEvent) {
  const { result } = app
  if (!app.server_id.startsWith('zoom:') || app.tool !== 'search_meetings' || result.isError) return
  const list = meetingList.safeParse(result.structuredContent)
  if (!list.success) return

  const joinable: ZoomJoinableMeeting[] = []
  const otherMeetings: unknown[] = []
  for (const row of list.data.meetings) {
    const meeting = upcomingMeeting.safeParse(row).data
    const url = meeting && participantLink(meeting.join_url)
    if (meeting && url) {
      joinable.push({ ...meeting, join_url: url.href })
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
