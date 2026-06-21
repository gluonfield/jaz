import type { ThreadSearchResult } from './api/types'

export function threadSearchTitle(result: Pick<ThreadSearchResult, 'thread_title'>): string {
  return result.thread_title || 'Untitled thread'
}
