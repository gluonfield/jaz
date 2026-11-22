import { expect, test } from 'bun:test'
import { zoomMeetingResults } from '@/lib/zoomMeetingResults'

const participant = 'https://us05web.zoom.us/j/12345678901?pwd=a%2Bb%3D%3D&omn=123'
const scheduled = {
  meeting_category: 'scheduled_upcoming',
  topic: 'Design review',
  schedule_start_time: '2027-03-15T09:00:00Z',
  schedule_end_time: '2027-03-15T09:40:00Z',
  host_name: 'Meeting host',
  join_url: participant,
}
const history = {
  meeting_category: 'history',
  topic: 'Last week’s review',
  meeting_uuid: 'past-meeting',
  has_transcript: true,
  join_url: participant,
}

function searchResult(meetings) {
  const structuredContent = Object.freeze({ from_server: true, meetings: Object.freeze(meetings) })
  return {
    server_id: 'zoom:https-mcp-zoom-us-mcp-zoom-streamable-98e72934',
    tool: 'search_meetings',
    result: Object.freeze({ content: [{ type: 'text', text: JSON.stringify(structuredContent) }], structuredContent }),
  }
}

test('scheduled meeting links preserve the passcode and history stays in the provider view', () => {
  const app = searchResult([scheduled, history])
  const view = zoomMeetingResults(app)
  expect(view.joinable).toEqual([scheduled])
  expect(view.providerResult.structuredContent).toEqual({ from_server: true, meetings: [history] })
  expect(app.result.structuredContent.meetings).toEqual([scheduled, history])
})

test('missing display details do not disable a valid join link', () => {
  const row = Object.freeze({
    meeting_category: 'scheduled_upcoming',
    join_url: participant,
    host_name: null,
    schedule_start_time: 0,
  })
  const app = searchResult([history, row])
  const view = zoomMeetingResults(app)
  expect(view.joinable).toEqual([{
    meeting_category: 'scheduled_upcoming',
    join_url: participant,
    topic: '',
    host_name: '',
    schedule_start_time: '',
    schedule_end_time: '',
  }])
  expect(view.providerResult.structuredContent.meetings).toEqual([history])
  expect(app.result.structuredContent.meetings).toEqual([history, row])
})

test('the Zoom meeting card leaves host links, missing links, and other providers to their existing views', () => {
  const invalid = [
    { ...scheduled, join_url: undefined, start_url: 'https://zoom.us/s/12345678901?zak=host' },
    { ...scheduled, join_url: 'https://zoom.us/s/12345678901?zak=host' },
    { ...scheduled, join_url: 'https://zoom.us.example.net/j/12345678901' },
    { ...scheduled, join_url: 'http://zoom.us/j/12345678901' },
  ]
  expect(zoomMeetingResults(searchResult(invalid))).toBeUndefined()
  expect(zoomMeetingResults({ ...searchResult([scheduled]), server_id: 'other' })).toBeUndefined()
  expect(zoomMeetingResults({ ...searchResult([scheduled]), server_id: 'plugin:zoom:em9vbQ' })?.joinable).toEqual([scheduled])
})
