import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { OverviewPanel } from '@/components/session/OverviewPanel'
import { ToastProvider } from '@/components/ui/toast'
import type { RepoInfo, Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export async function exerciseRepoStatus(): Promise<void> {
  const originalFetch = window.fetch
  const size = { width: innerWidth, height: innerHeight }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let info: RepoInfo = { git: true, owner: 'example', repo: 'repo', branch: 'jaz/feature', default_branch: 'develop', main_branch: 'main', is_worktree: true, ahead: 1, no_commits: true, has_upstream: true, web_url: 'https://github.com/example/repo' }
  let failed = false
  let merges = 0
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.endsWith('/v1/sessions/repo-status/repo/merge')) {
      if (init?.method !== 'POST') throw new Error('Merge did not use the repo action')
      merges += 1
      info = { ...info, ahead: 0 }
      return Response.json({ cwd: '/fixture/repo', info })
    }
    if (url.endsWith('/v1/sessions/repo-status/repo')) {
      return failed ? Response.json({ error: 'Git status failed' }, { status: 500 }) : Response.json(info)
    }
    if (url.endsWith('/v1/skills')) return Response.json({ skills: [] })
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;z-index:1;overflow:auto;padding:24px;background:var(--color-bg)'
  document.body.append(element)
  const root = createRoot(element)
  const session: Session = { id: 'repo-status', slug: 'repo-status', status: 'idle', runtime: 'acp', runtime_ref: { type: 'acp', cwd: '/fixture/worktree' }, created_at: '', updated_at: '', last_attention_at: '' }
  const noop = async () => {}
  const button = (label: string) => [...element.querySelectorAll('button')].find((item) => item.textContent?.trim() === label)
  const until = async (check: () => boolean) => {
    const end = Date.now() + 5000
    while (!check()) {
      if (Date.now() > end) throw new Error('Repo status did not settle: ' + check.toString())
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const refresh = () => client.invalidateQueries({ queryKey: keys.sessionRepo(session.id), exact: true })
  try {
    root.render(<QueryClientProvider client={client}><ToastProvider><OverviewPanel session={session} subagents={[]} spawnedThreads={[]} working={false} onSend={noop} onQueuePrompt={noop} onQueueAction={noop} /></ToastProvider></QueryClientProvider>)
    await until(() => Boolean(button('Merge into main')))
    if (button('Merge into main')!.disabled || !element.textContent?.includes('1 commit is not in main.') || element.textContent.includes('Working tree clean')) {
      throw new Error('A pushed branch was presented as merged or its merge action was hidden')
    }
    await window.smoke.capture('repo-unmerged-branch')
    button('Merge into main')!.click()
    await until(() => Boolean(merges === 1 && button('Merge into main')?.disabled))
    if (!element.textContent?.includes('No commits to merge from this branch into main.') || !document.body.textContent?.includes('Push and release are separate.')) {
      throw new Error('A local merge hid the remaining publication steps')
    }
    info = { ...info, ahead: undefined }
    await refresh()
    await until(() => Boolean(element.textContent?.includes('Merge status against main is unknown.')))
    if (button('Merge into main')!.disabled) throw new Error('An unknown merge status was treated as zero commits')
    failed = true
    await refresh()
    await until(() => Boolean(button('Retry Git status')))
    if (button('Merge into main') || element.textContent?.includes('No uncommitted changes')) {
      throw new Error('Failed Git status left stale success information or merge controls visible')
    }
    failed = false
    info = { ...info, ahead: 2 }
    button('Retry Git status')!.click()
    await until(() => Boolean(element.textContent?.includes('2 commits are not in main.')))
    await window.smoke.resize(390, 844)
    await new Promise(requestAnimationFrame)
    if (element.scrollWidth > element.clientWidth) throw new Error('Repo status overflows the phone viewport')
    await window.smoke.capture('repo-unmerged-branch-phone')
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    await window.smoke.resize(size.width, size.height)
  }
}
