import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRoot } from 'react-dom/client'
import { CodeDiffPanel } from '@/components/session/CodeDiffPanel'
import { OverviewPanel } from '@/components/session/OverviewPanel'
import { ToastProvider } from '@/components/ui/toast'
import type { RepoChanges, RepoInfo, Session } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export async function exerciseRepoStatus(): Promise<void> {
  const originalFetch = window.fetch
  const size = { width: innerWidth, height: innerHeight }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let info: RepoInfo = { git: true, owner: 'example', repo: 'repo', branch: 'jaz/feature', default_branch: 'develop', main_branch: 'main', is_worktree: true, ahead: 1, dirty: false, head: '1234567890abcdef', upstream: 'origin/jaz/feature', push_ahead: 0, no_commits: true, has_upstream: true, web_url: 'https://github.com/example/repo' }
  let failed = false
  let merges = 0
  let diffFailed = false
  let changes: RepoChanges = { base: 'abcdef1234567890abcdef1234567890abcdef12', base_kind: 'fork', files: [{ path: 'feature.txt', status: 'modified', added: 1, deleted: 0 }, { path: 'dirty.txt', status: 'untracked', added: 1, deleted: 0 }], total_added: 2, total_deleted: 0 }
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.endsWith('/v1/sessions/repo-status/repo/changes')) {
      return diffFailed ? Response.json({ error: 'Git diff failed' }, { status: 502 }) : Response.json(changes)
    }
    if (url.includes('/v1/sessions/repo-status/repo/diff?')) {
      return Response.json({ path: 'feature.txt', patch: 'diff --git a/feature.txt b/feature.txt\n--- a/feature.txt\n+++ b/feature.txt\n@@ -1 +1 @@\n-old\n+new' })
    }
    if (url.endsWith('/v1/sessions/repo-status/repo/merge')) {
      if (init?.method !== 'POST') throw new Error('Merge did not use the repo action')
      merges += 1
      info = { ...info, ahead: 0, dirty: false }
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
    root.render(<QueryClientProvider client={client}><ToastProvider><div style={{ width: 256 }}><OverviewPanel session={session} subagents={[]} spawnedThreads={[]} working={false} onSend={noop} onQueuePrompt={noop} onQueueAction={noop} /></div></ToastProvider></QueryClientProvider>)
    await until(() => Boolean(button('Merge into main')))
    if (button('Merge into main')!.disabled || !element.textContent?.includes('1 commit outside') || element.textContent.includes('Working tree clean')) {
      throw new Error('A pushed branch was presented as merged or its merge action was hidden')
    }
    for (const value of element.querySelectorAll('dt, dd, dd span')) {
      if (value.scrollWidth > value.clientWidth) throw new Error('Git status is truncated at sidebar width: ' + value.textContent)
    }
    await window.smoke.capture('repo-unmerged-branch')
    info = { ...info, dirty: true }
    await refresh()
    await until(() => Boolean(button('Commit changes')))
    if (!element.textContent?.includes('1 commit outside')) throw new Error('Dirty edits hid outstanding commits')
    button('Merge into main')!.click()
    await until(() => Boolean(merges === 1 && button('Merge into main')?.disabled))
    if (!element.textContent?.includes('Commits included') || !document.body.textContent?.includes('Push and release are separate.')) {
      throw new Error('A local merge hid the remaining publication steps')
    }
    info = { ...info, ahead: undefined }
    await refresh()
    await until(() => Boolean(element.textContent?.includes('Local mainUnknown')))
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
    await until(() => Boolean(element.textContent?.includes('2 commits outside')))
    for (const dismiss of document.querySelectorAll<HTMLButtonElement>('button[aria-label="Dismiss notification"]')) dismiss.click()
    await until(() => !document.querySelector('button[aria-label="Dismiss notification"]'))
    await window.smoke.resize(390, 844)
    await new Promise(requestAnimationFrame)
    if (element.scrollWidth > element.clientWidth) throw new Error('Repo status overflows the phone viewport')
    await window.smoke.capture('repo-unmerged-branch-phone')
    info = { git: true, branch: 'main', head: 'feedface12345678', dirty: false, upstream: 'origin/main', push_ahead: 0 }
    await refresh()
    await until(() => Boolean(element.textContent?.includes('WorkspaceLocal checkout')))
    if (button('Merge into main') || element.textContent?.includes('Commits included')) throw new Error('Clean local main implied task inclusion')
    await window.smoke.capture('repo-local-checkout-sidebar')
    info = { ...info, dirty: undefined }
    await refresh()
    await until(() => Boolean(element.textContent?.includes('ChangesUnknown')))
    await window.smoke.resize(size.width, size.height)
    info = { git: true, branch: 'jaz/feature', is_worktree: true, main_branch: 'main', dirty: true }
    await refresh()
    root.render(<QueryClientProvider client={client}><CodeDiffPanel sessionId={session.id} visible /></QueryClientProvider>)
    await until(() => Boolean(element.textContent?.includes('Merge base abcdef1 → working tree') && element.textContent.includes('dirty.txt')))
    if (element.textContent?.includes('main →')) throw new Error('A branch label replaced the resolved diff base')
    await window.smoke.capture('repo-branch-diff')
    info = { git: true, branch: 'main', head: 'feedface12345678', dirty: false }
    await refresh()
    changes = { base: 'feedface12345678feedface12345678feedface', base_kind: 'head', files: [], total_added: 0, total_deleted: 0 }
    await client.invalidateQueries({ queryKey: keys.sessionRepoChanges(session.id) })
    await until(() => Boolean(element.textContent?.includes('HEAD feedfac → working tree') && element.textContent.includes('No uncommitted changes in this checkout.')))
    diffFailed = true
    await client.invalidateQueries({ queryKey: keys.sessionRepoChanges(session.id) })
    await until(() => Boolean(button('Retry code changes')))
    if (element.textContent?.includes('No uncommitted changes in this checkout.')) throw new Error('A failed diff became an empty-success state')
    await window.smoke.capture('repo-diff-error')
    diffFailed = false
    button('Retry code changes')!.click()
    await until(() => Boolean(element.textContent?.includes('No uncommitted changes in this checkout.')))
  } finally {
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
    await window.smoke.resize(size.width, size.height)
  }
}
