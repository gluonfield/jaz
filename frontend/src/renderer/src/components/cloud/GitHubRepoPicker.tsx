import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ChevronDown, GitBranch, LoaderCircle, Lock, Plus, Search, X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Popover } from '@/components/ui/Popover'
import { useToast } from '@/components/ui/toast'
import {
  useCloudGitHubBranches,
  useCloudGitHubOAuth,
  cloudGitHubReposQuery,
  useCloudGitHubStatus,
  withGitHubPluginSearch,
} from '@/lib/api/cloudGitHub'
import type { CloudGitRepoSpec } from '@/lib/api/cloudThreadRepos'
import { useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

interface PickerProps {
  value: CloudGitRepoSpec[]
  includePersonal: boolean
  disabled?: boolean
}

export function GitHubBranchField({
  repo, value, disabled, onChange,
}: {
  repo: string
  value: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  const id = useId()
  const branches = useCloudGitHubBranches(repo, !disabled)
  return (
    <label className="block min-w-0">
      <span className="mb-1 block text-[11px] text-ink-3">Base branch</span>
      <Input
        aria-label={`Base branch for ${repo}`}
        value={value}
        list={id}
        placeholder={branches.isLoading ? 'Loading branches…' : 'Default branch'}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="h-8 text-[12px]"
      />
      <datalist id={id}>
        {branches.data?.branches.map(({ name }) => <option key={name} value={name} />)}
      </datalist>
    </label>
  )
}

export function ThreadRepoPicker({
  value, includePersonal, disabled, placement, onChange,
}: PickerProps & {
  placement?: 'above' | 'below'
  onChange: (repos: CloudGitRepoSpec[]) => void
}) {
  return (
    <RepositoryMenu
      value={value}
      includePersonal={includePersonal}
      disabled={disabled}
      placement={placement}
      multiple
      onSelect={(next) => {
        const selected = value.some(({ repo }) => sameRepo(repo, next.repo))
        onChange(selected ? value.filter(({ repo }) => !sameRepo(repo, next.repo)) : [...value, next])
      }}
    >
      {value.length > 0 ? (
        <div className="space-y-2 border-t border-border pt-2">
          <RepositoryRows value={value} onChange={onChange} />
          <Button variant="ghost" size="sm" onClick={() => onChange([])}>Clear selection</Button>
        </div>
      ) : null}
    </RepositoryMenu>
  )
}

export function GitHubRepoField({
  value, disabled, includePersonal, onChange,
}: PickerProps & { onChange: (repos: CloudGitRepoSpec[]) => void }) {
  return (
    <div className="space-y-2">
      <p className="text-[12px] font-medium text-ink-2">Default repositories</p>
      <RepositoryRows value={value} disabled={disabled} onChange={onChange} />
      {!disabled ? (
        <AddGitHubRepository value={value} includePersonal={includePersonal} onSelect={(repo) => onChange([...value, repo])} />
      ) : null}
    </div>
  )
}

export function AddGitHubRepository({
  value, includePersonal, disabled, onSelect,
}: PickerProps & { onSelect: (repo: CloudGitRepoSpec) => void }) {
  return (
    <RepositoryMenu
      value={value}
      includePersonal={includePersonal}
      disabled={disabled}
      onSelect={onSelect}
    />
  )
}

function RepositoryRows({
  value, disabled, onChange,
}: Pick<PickerProps, 'value' | 'disabled'> & { onChange: (repos: CloudGitRepoSpec[]) => void }) {
  return value.map((item) => (
    <div key={item.repo} className="flex items-end gap-2">
      <span className="min-w-0 flex-1 truncate py-2 text-[12px] text-ink-2" title={item.repo}>{item.repo}</span>
      <div className="min-w-0 flex-1">
        <GitHubBranchField
          repo={item.repo}
          value={item.base_branch ?? ''}
          disabled={disabled}
          onChange={(base_branch) => onChange(value.map((repo) => repo === item ? { ...repo, base_branch } : repo))}
        />
      </div>
      {!disabled ? (
        <button
          type="button"
          className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-control text-ink-3 hover:bg-surface-2 hover:text-ink"
          aria-label={`Remove ${item.repo}`}
          onClick={() => onChange(value.filter((repo) => repo !== item))}
        >
          <X size={14} />
        </button>
      ) : null}
    </div>
  ))
}

function RepositoryMenu({
  value, disabled, includePersonal, placement, multiple, onSelect, children,
}: PickerProps & {
  placement?: 'above' | 'below'
  multiple?: boolean
  onSelect: (repo: CloudGitRepoSpec) => void
  children?: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const navigate = useNavigate()
  const toast = useToast()
  const isViewer = useIsCloudViewer()
  const showing = open && !disabled
  const oauth = useCloudGitHubOAuth({
    onStarted: () => toast('Finish GitHub sign-in in your browser'),
    onError: (err) => toast(err.message || 'Could not start GitHub sign-in', 'danger'),
  })
  const queryClient = useQueryClient()
  const status = useCloudGitHubStatus(showing)
  const repos = useQuery({
    ...cloudGitHubReposQuery('', includePersonal),
    enabled: showing && Boolean(status.data?.configured),
    refetchInterval: oauth.polling ? 2_000 : false,
  })

  useEffect(() => {
    if (oauth.polling && repos.data?.user_linked) {
      oauth.stopPolling()
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
    }
  }, [oauth, queryClient, repos.data?.user_linked])

  const selected = new Set(value.map(({ repo }) => repo.toLowerCase()))
  const filter = search.trim().toLowerCase()
  const matches = (repos.data?.repos ?? []).filter(({ full_name }) => {
    const name = full_name.toLowerCase()
    return name.includes(filter) && (multiple || !selected.has(name))
  })
  const Icon = multiple ? GitBranch : Plus
  const label = multiple
    ? value.slice(0, 2).map(({ repo }) => repo.split('/').at(-1)).join(', ') || 'Repositories'
    : 'Add repository'
  const close = () => {
    setOpen(false)
    setSearch('')
  }
  const manage = () => {
    close()
    void navigate({ to: '.', search: withGitHubPluginSearch })
  }

  const trigger = (
    <button
      ref={triggerRef}
      type="button"
      disabled={disabled}
      aria-expanded={showing}
      aria-label={multiple ? 'Select repositories' : label}
      title={multiple && value.length ? value.map(({ repo }) => repo).join(', ') : label}
      onClick={() => showing ? close() : setOpen(true)}
      className="inline-flex h-7 max-w-[15rem] cursor-pointer items-center gap-1.5 rounded-full px-2.5 text-[12px] font-medium text-ink-2 transition-[background-color,color,scale] hover:bg-surface-2 hover:text-ink active:scale-[0.96] disabled:cursor-default disabled:opacity-50"
    >
      <Icon size={13} className="shrink-0" />
      <span className="truncate">{label}</span>
      {multiple && value.length > 2 ? <span className="shrink-0 text-ink-3 tabular-nums">+{value.length - 2}</span> : null}
      <ChevronDown size={11} className={`shrink-0 transition-transform ${showing ? 'rotate-180' : ''}`} />
    </button>
  )
  const content = (
    <div className="flex max-h-[min(55dvh,28rem)] w-full min-w-0 flex-col gap-2 overflow-y-auto p-1.5">
      {status.isPending || (status.data?.configured && repos.isPending) ? (
        <p className="flex items-center gap-2 px-2 py-2 text-[12px] text-ink-3">
          <LoaderCircle size={13} className="animate-spin" />
          {status.isPending ? 'Checking GitHub…' : 'Loading repositories…'}
        </p>
      ) : status.isError || repos.isError ? (
        <div className="px-2 text-[12px]">
          <p className="text-danger">Couldn’t load GitHub repositories.</p>
          <Button variant="ghost" size="sm" onClick={() => void (status.isError ? status.refetch() : repos.refetch())}>Retry</Button>
        </div>
      ) : !status.data?.configured || repos.data?.connect_required ? (
        <div className="space-y-1 px-2 text-[12px] text-ink-2">
          <p>{status.data?.configured ? 'Connect GitHub to browse repositories.' : 'GitHub is unavailable in this workspace.'}</p>
          {repos.data?.connect_required && !isViewer ? (
            <Button variant="primary" size="sm" disabled={oauth.isPending || oauth.polling} onClick={() => oauth.mutate(undefined)}>
              {oauth.polling ? 'Waiting for GitHub…' : 'Connect GitHub'}
            </Button>
          ) : null}
          <Button variant="ghost" size="sm" onClick={manage}>Manage in Plugins</Button>
        </div>
      ) : (
        <>
          <div className="relative">
            <Search size={13} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" />
            <Input
              autoFocus
              aria-label="Filter repositories"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Filter repositories…"
              className="h-8 pl-7 text-[12px]"
            />
          </div>
          <div className="max-h-44 shrink-0 overflow-y-auto">
            {matches.map(({ full_name, default_branch, private: isPrivate }) => {
              const pick = () => {
                onSelect({ repo: full_name, base_branch: default_branch })
                if (!multiple) {
                  close()
                  triggerRef.current?.focus()
                }
              }
              const content = (
                <>
                  {isPrivate ? <Lock size={11} className="shrink-0 text-ink-3" aria-label="Private" /> : null}
                  <span className="truncate">{full_name}</span>
                </>
              )
              const className = 'flex min-h-9 w-full cursor-pointer items-center gap-2 rounded-control px-2 text-left text-[12px] text-ink-2 hover:bg-surface-2'
              return multiple ? (
                <label key={full_name} className={className} title={full_name}>
                  <input
                    type="checkbox"
                    checked={selected.has(full_name.toLowerCase())}
                    onChange={pick}
                    className="accent-primary"
                  />
                  {content}
                </label>
              ) : (
                <button key={full_name} type="button" className={className} title={full_name} onClick={pick}>{content}</button>
              )
            })}
            {!matches.length ? (
              <p className="px-2 py-2 text-[12px] text-ink-3">
                {filter ? 'No matching repositories.' : 'No additional repositories available.'}
              </p>
            ) : null}
          </div>
        </>
      )}
      {children}
    </div>
  )

  if (multiple) {
    return (
      <Popover open={showing} onClose={close} placement={placement} trigger={trigger}>
        <div className="w-[min(330px,calc(100vw-32px))]">{content}</div>
      </Popover>
    )
  }
  return (
    <div>
      {trigger}
      {showing ? <div className="rounded-control bg-surface-2/50">{content}</div> : null}
    </div>
  )
}

function sameRepo(left: string, right: string) {
  return left.toLowerCase() === right.toLowerCase()
}
