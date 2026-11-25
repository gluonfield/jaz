import { useQueryClient } from '@tanstack/react-query'
import { ChevronDown, GitBranch, LoaderCircle, Lock, LogIn, Search, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import {
  useCloudGitHubBranches,
  useCloudGitHubOAuth,
  useCloudGitHubRepos,
  useCloudGitHubStatus,
} from '@/lib/api/cloudGitHub'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

function useGitHubPickerData({
  enabled,
  loadRepos,
  repo,
  includePersonal,
}: {
  enabled: boolean
  loadRepos: boolean
  repo: string
  includePersonal: boolean
}) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const oauth = useCloudGitHubOAuth({
    onStarted: () => toast('Finish GitHub sign-in in your browser'),
    onError: (err) => toast(err.message || 'Could not start GitHub sign-in', 'danger'),
  })
  const status = useCloudGitHubStatus(enabled, oauth.polling)
  const configured = Boolean(status.data?.configured)
  const repos = useCloudGitHubRepos({ includePersonal, enabled: loadRepos && configured })
  const branches = useCloudGitHubBranches(repo, loadRepos && configured && Boolean(repo))

  useEffect(() => {
    if (oauth.polling && status.data?.user_linked) {
      oauth.stopPolling()
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
    }
  }, [oauth, queryClient, status.data?.user_linked])

  return { status, repos, branches, oauth }
}

// Compact shelf/inline repository + base-branch selector for New Thread,
// Project New Chat, and attaching a repo to an existing Cloud thread.
export function ThreadRepoPicker({
  repo,
  baseBranch,
  placement = 'above',
  disabled,
  includePersonal = true,
  placeholder = 'Repository',
  onChange,
}: {
  repo: string
  baseBranch: string
  placement?: 'above' | 'below'
  disabled?: boolean
  includePersonal?: boolean
  placeholder?: string
  onChange: (next: { repo: string; baseBranch: string }) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const isViewer = useIsCloudViewer()

  const { status, repos, branches, oauth } = useGitHubPickerData({
    enabled: !disabled,
    loadRepos: open,
    repo,
    includePersonal,
  })

  const filteredRepos = useMemo(() => {
    const items = repos.data?.repos ?? []
    const q = search.trim().toLowerCase()
    return q ? items.filter((item) => item.full_name.toLowerCase().includes(q)) : items
  }, [repos.data?.repos, search])

  const selectedRepoMeta = repos.data?.repos.find(
    (item) => item.full_name.toLowerCase() === repo.trim().toLowerCase(),
  )
  const effectiveBranch = baseBranch || selectedRepoMeta?.default_branch || ''
  const shortRepo = repo ? (repo.split('/').at(-1) || repo) : placeholder
  const label = repo ? (effectiveBranch ? `${shortRepo}:${effectiveBranch}` : shortRepo) : placeholder

  const selectRepo = (fullName: string, defaultBranch: string) => {
    setSearch('')
    onChange({ repo: fullName, baseBranch: defaultBranch })
    if (!fullName) setOpen(false)
  }

  return (
    <Popover
      open={open && !disabled}
      onClose={() => setOpen(false)}
      placement={placement}
      align="start"
      trigger={
        <button
          type="button"
          disabled={disabled}
          aria-haspopup="dialog"
          aria-expanded={open && !disabled}
          aria-label={repo ? `Repository: ${repo}${effectiveBranch ? ` (${effectiveBranch})` : ''}` : placeholder}
          title={repo ? `${repo}${effectiveBranch ? ` (${effectiveBranch})` : ''}` : placeholder}
          onClick={() => setOpen((value) => !value)}
          className="inline-flex h-7 max-w-[15rem] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] duration-150 hover:bg-surface-2 hover:text-ink active:scale-[0.96] disabled:cursor-default disabled:opacity-50"
        >
          <GitBranch size={13} className="shrink-0" />
          <span className="truncate">{label}</span>
          <ChevronDown size={11} className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''}`} />
        </button>
      }
    >
      <div className="flex w-[280px] flex-col gap-2 p-1.5">
        {status.isPending ? (
          <div className="flex items-center gap-2 px-2 py-2 text-[12px] text-ink-3">
            <LoaderCircle size={13} className="animate-spin" />
            Checking GitHub…
          </div>
        ) : !status.data?.configured ? (
          <p className="px-2 py-1.5 text-[12px] text-ink-3">
            GitHub App is not configured for this workspace yet. Set it up in Settings → GitHub.
          </p>
        ) : repos.data?.connect_required ? (
          <div className="flex flex-col gap-2 px-2 py-1.5">
            <p className="text-[12px] text-ink-2">Connect your GitHub account to browse repositories.</p>
            {!isViewer ? (
              <Button
                variant="primary"
                size="sm"
                disabled={oauth.isPending}
                onClick={() => oauth.mutate(undefined)}
              >
                {oauth.isPending ? <LoaderCircle size={13} className="animate-spin" /> : <LogIn size={13} />}
                Connect GitHub
              </Button>
            ) : null}
          </div>
        ) : (
          <>
            <div className="relative">
              <Search size={13} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" />
              <Input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Filter repositories…"
                className="h-7 pl-7 text-[12px]"
              />
            </div>

            <div className="max-h-[190px] overflow-y-auto">
              <MenuRow role="menuitemradio" selected={!repo} onClick={() => selectRepo('', '')}>
                No repository
              </MenuRow>
              {repos.isPending ? (
                <div className="flex items-center gap-2 px-2.5 py-2 text-[12px] text-ink-3">
                  <LoaderCircle size={12} className="animate-spin" />
                  Loading repositories…
                </div>
              ) : filteredRepos.length === 0 ? (
                <p className="px-2.5 py-2 text-[12px] text-ink-3">No matching repositories.</p>
              ) : (
                filteredRepos.map((item) => (
                  <MenuRow
                    key={item.full_name}
                    role="menuitemradio"
                    selected={item.full_name.toLowerCase() === repo.trim().toLowerCase()}
                    onClick={() => selectRepo(item.full_name, item.default_branch)}
                  >
                    <span className="flex min-w-0 flex-1 items-center gap-1.5">
                      {item.private ? <Lock size={11} className="shrink-0 text-ink-3" /> : null}
                      <span className="truncate">{item.full_name}</span>
                    </span>
                  </MenuRow>
                ))
              )}
            </div>

            {repo ? (
              <div className="border-t border-border/70 pt-2">
                <label className="block px-1">
                  <span className="mb-1 block text-[11px] font-medium text-ink-3">Base branch</span>
                  {branches.isPending ? (
                    <div className="flex items-center gap-1.5 py-1 text-[12px] text-ink-3">
                      <LoaderCircle size={12} className="animate-spin" />
                      Loading branches…
                    </div>
                  ) : branches.data && branches.data.branches.length > 0 ? (
                    <select
                      aria-label="Base branch"
                      value={baseBranch}
                      onChange={(e) => {
                        onChange({ repo, baseBranch: e.target.value })
                        setOpen(false)
                      }}
                      className="h-7 w-full rounded-control border border-border bg-bg px-2 text-[12px] text-ink focus:border-primary focus:outline-none"
                    >
                      {!branches.data.branches.some((b) => b.name === baseBranch) && baseBranch ? (
                        <option value={baseBranch}>{baseBranch}</option>
                      ) : null}
                      {branches.data.branches.map((branch) => (
                        <option key={branch.name} value={branch.name}>
                          {branch.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <Input
                      type="text"
                      value={baseBranch}
                      onChange={(e) => onChange({ repo, baseBranch: e.target.value })}
                      placeholder={selectedRepoMeta?.default_branch || 'main'}
                      className="h-7 text-[12px]"
                    />
                  )}
                </label>
              </div>
            ) : null}
          </>
        )}
      </div>
    </Popover>
  )
}

// Form-layout default repository + base branch fields for Space (Project) settings.
export function GitHubRepoField({
  repo,
  baseBranch,
  disabled,
  includePersonal = true,
  onChange,
}: {
  repo: string
  baseBranch: string
  disabled?: boolean
  includePersonal?: boolean
  onChange: (next: { repo: string; baseBranch: string }) => void
}) {
  const isViewer = useIsCloudViewer()
  const { status, repos, branches, oauth } = useGitHubPickerData({
    enabled: true,
    loadRepos: true,
    repo,
    includePersonal,
  })

  if (!status.isLoading && !status.data?.configured && !repo) {
    return null
  }

  const repoList = repos.data?.repos ?? []
  const branchList = branches.data?.branches ?? []

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[12px] font-medium text-ink-2">Default repository</span>
        {repo && !disabled ? (
          <button
            type="button"
            onClick={() => onChange({ repo: '', baseBranch: '' })}
            className="inline-flex cursor-pointer items-center gap-1 text-[11.5px] text-ink-3 hover:text-ink"
          >
            <X size={11} />
            Clear
          </button>
        ) : null}
      </div>

      {repos.data?.connect_required ? (
        <div className="flex items-center justify-between gap-2 rounded-control border border-border bg-surface px-2.5 py-2 text-[12px] text-ink-2">
          <span>Connect GitHub to select a repository.</span>
          {!isViewer && !disabled ? (
            <Button
              variant="secondary"
              size="sm"
              disabled={oauth.isPending}
              onClick={() => oauth.mutate(undefined)}
            >
              {oauth.isPending ? <LoaderCircle size={12} className="animate-spin" /> : <LogIn size={12} />}
              Connect
            </Button>
          ) : null}
        </div>
      ) : (
        <div className="grid gap-2 sm:grid-cols-2">
          <select
            aria-label="Default repository"
            value={repo}
            disabled={disabled || repos.isLoading}
            onChange={(e) => {
              const nextRepo = e.target.value
              const match = repoList.find((item) => item.full_name === nextRepo)
              onChange({ repo: nextRepo, baseBranch: match?.default_branch ?? '' })
            }}
            className="h-9 w-full rounded-control border border-border bg-bg px-2.5 text-[13px] text-ink focus:border-primary focus:outline-none disabled:opacity-60"
          >
            <option value="">None</option>
            {repo && !repoList.some((item) => item.full_name === repo) ? (
              <option value={repo}>{repo}</option>
            ) : null}
            {repoList.map((item) => (
              <option key={item.full_name} value={item.full_name}>
                {item.full_name}
              </option>
            ))}
          </select>

          {branchList.length > 0 ? (
            <select
              aria-label="Default base branch"
              value={baseBranch}
              disabled={disabled || !repo || branches.isLoading}
              onChange={(e) => onChange({ repo, baseBranch: e.target.value })}
              className="h-9 w-full rounded-control border border-border bg-bg px-2.5 text-[13px] text-ink focus:border-primary focus:outline-none disabled:opacity-60"
            >
              <option value="">Default branch</option>
              {baseBranch && !branchList.some((b) => b.name === baseBranch) ? (
                <option value={baseBranch}>{baseBranch}</option>
              ) : null}
              {branchList.map((branch) => (
                <option key={branch.name} value={branch.name}>
                  {branch.name}
                </option>
              ))}
            </select>
          ) : (
            <Input
              type="text"
              aria-label="Default base branch"
              value={baseBranch}
              disabled={disabled || !repo}
              onChange={(e) => onChange({ repo, baseBranch: e.target.value })}
              placeholder={repo ? 'Default branch (e.g. main)' : 'Base branch'}
            />
          )}
        </div>
      )}
      <p className="text-[11.5px] text-ink-3">
        New chats started in this project inherit this repository and base branch automatically.
      </p>
    </div>
  )
}
