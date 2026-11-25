import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  ExternalLink,
  GitBranch,
  LoaderCircle,
  LogIn,
  Plus,
  Trash2,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { SettingsCard } from '@/components/settings/SettingsCard'
import { Button } from '@/components/ui/Button'
import { Collapse } from '@/components/ui/Collapse'
import { Input } from '@/components/ui/Input'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { useToast } from '@/components/ui/toast'
import {
  DEFAULT_GITHUB_APP_NAME,
  deleteCloudGitHubInstallation,
  disconnectCloudGitHubAccount,
  isValidGitHubSegment,
  openSafeGitHubUrl,
  patchCloudGitHubInstallation,
  splitRepoFullName,
  startCloudGitHubInstall,
  startCloudGitHubManifest,
  submitGitHubManifestForm,
  useCloudGitHubAuthRefresh,
  useCloudGitHubOAuth,
  useCloudGitHubStatus,
  type CloudGitHubInstallation,
} from '@/lib/api/cloudGitHub'
import { useCloudSession, useIsCloudViewer } from '@/lib/cloudSession'
import { keys } from '@/lib/query/keys'

function parseLineList(raw: string): string[] {
  return raw
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function CloudGitHubSettings() {
  const cloud = useCloudSession()
  const isViewer = useIsCloudViewer()
  const isAdmin = !isViewer && (cloud.user?.role ?? '').trim().toLowerCase() === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()

  const authRefresh = useCloudGitHubAuthRefresh()
  const statusQuery = useCloudGitHubStatus(cloud.active, authRefresh.polling)
  const status = statusQuery.data

  // Stop 2s polling as soon as an expected GitHub setup transition completes.
  const prevStateRef = useRef<{ configured: boolean; linked: boolean; count: number } | null>(null)
  useEffect(() => {
    if (!status) return
    const next = {
      configured: status.configured,
      linked: status.user_linked,
      count: status.installations.length,
    }
    const prev = prevStateRef.current
    prevStateRef.current = next
    if (
      prev &&
      authRefresh.polling &&
      ((!prev.configured && next.configured) || (!prev.linked && next.linked) || next.count > prev.count)
    ) {
      authRefresh.stop()
    }
  }, [authRefresh, status])

  const [appName, setAppName] = useState(DEFAULT_GITHUB_APP_NAME)
  const [githubOrg, setGithubOrg] = useState('')
  const [showAdvancedManifest, setShowAdvancedManifest] = useState(false)

  const startManifest = useMutation({
    mutationFn: () => {
      const org = githubOrg.trim()
      if (org && !isValidGitHubSegment(org)) {
        return Promise.reject(new Error('GitHub organization slug contains invalid characters'))
      }
      return startCloudGitHubManifest({
        app_name: appName.trim() || DEFAULT_GITHUB_APP_NAME,
        github_org: org || undefined,
      })
    },
    onSuccess: (res) => {
      submitGitHubManifestForm(res.action_url, res.manifest)
      authRefresh.start()
      toast('Finish creating the GitHub App in your browser')
    },
    onError: (err: Error) => {
      toast(err.message || 'Could not start GitHub App setup', 'danger')
    },
  })

  const startInstall = useMutation({
    mutationFn: (scope: 'org' | 'user') => startCloudGitHubInstall(scope),
    onSuccess: (res) => {
      openSafeGitHubUrl(res.install_url)
      authRefresh.start()
      toast('Complete installation in GitHub')
    },
    onError: (err: Error) => {
      toast(err.message || 'Could not start GitHub installation', 'danger')
    },
  })

  const oauth = useCloudGitHubOAuth({
    onStarted: () => {
      authRefresh.start()
      toast('Finish GitHub sign-in in your browser')
    },
    onError: (err) => {
      toast(err.message || 'Could not start GitHub sign-in', 'danger')
    },
  })

  const disconnectAccount = useMutation({
    mutationFn: () => disconnectCloudGitHubAccount(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
      void queryClient.invalidateQueries({ queryKey: keys.plugins })
      toast('Disconnected GitHub account')
    },
    onError: (err: Error) => {
      toast(err.message || 'Could not disconnect GitHub account', 'danger')
    },
  })

  return (
    <section className="flex flex-col gap-5 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">GitHub</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">
          Connect a GitHub App and your GitHub account so Cloud threads can browse repositories, push branches, and open pull requests.
        </p>
      </div>

      {isViewer ? (
        <p className="rounded-card border border-border bg-surface px-3.5 py-2.5 text-[12.5px] text-ink-3">
          View-only access — viewers can inspect GitHub status but cannot change App settings, installations, or linked accounts.
        </p>
      ) : null}

      {statusQuery.isLoading ? (
        <SkeletonRows count={3} />
      ) : statusQuery.isError ? (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-card border border-danger/30 bg-surface px-3.5 py-3 text-[13px] text-danger">
          <span>{statusQuery.error instanceof Error ? statusQuery.error.message : 'Could not load GitHub status.'}</span>
          <Button variant="ghost" size="sm" onClick={() => void statusQuery.refetch()}>
            Retry
          </Button>
        </div>
      ) : status ? (
        <div className="flex flex-col gap-4">
          <SettingsCard className="p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <GitBranch size={15} className="shrink-0 text-ink-2" />
                  <h2 className="text-[14px] font-semibold text-ink">GitHub App</h2>
                  {status.configured ? (
                    <span className="inline-flex items-center gap-1 rounded-full bg-ok/12 px-2 py-0.5 text-[11px] font-medium text-ok">
                      <CheckCircle2 size={11} />
                      Configured
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-medium text-ink-3">
                      Not configured
                    </span>
                  )}
                </div>
                {status.configured ? (
                  <p className="mt-1 text-[12.5px] text-ink-2">
                    <span className="font-medium text-ink">{status.app_name || DEFAULT_GITHUB_APP_NAME}</span>
                    {status.app_slug ? <span className="ml-1.5 font-mono text-[12px] text-ink-3">({status.app_slug})</span> : null}
                  </p>
                ) : (
                  <p className="mt-1 text-[12.5px] text-ink-3">
                    {isAdmin
                      ? 'Create a GitHub App in one click with least-privilege permissions and automatic webhook setup.'
                      : 'Ask a workspace admin to configure the GitHub App before connecting repositories.'}
                  </p>
                )}
              </div>

              {status.configured && !isViewer ? (
                <div className="flex flex-wrap items-center gap-2">
                  {isAdmin ? (
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={startInstall.isPending}
                      onClick={() => startInstall.mutate('org')}
                    >
                      {startInstall.isPending && startInstall.variables === 'org' ? (
                        <LoaderCircle size={13} className="animate-spin" />
                      ) : (
                        <Plus size={13} />
                      )}
                      Install on organization
                    </Button>
                  ) : null}
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={startInstall.isPending}
                    onClick={() => startInstall.mutate('user')}
                  >
                    {startInstall.isPending && startInstall.variables === 'user' ? (
                      <LoaderCircle size={13} className="animate-spin" />
                    ) : (
                      <Plus size={13} />
                    )}
                    Install on personal account
                  </Button>
                  {status.install_manage_url ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openSafeGitHubUrl(status.install_manage_url!)}
                    >
                      Manage on GitHub
                      <ExternalLink size={12} />
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>

            {!status.configured && isAdmin ? (
              <div className="mt-4 flex flex-col gap-3 border-t border-border/70 pt-3.5">
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="primary"
                    size="md"
                    disabled={startManifest.isPending}
                    onClick={() => startManifest.mutate()}
                  >
                    {startManifest.isPending ? <LoaderCircle size={14} className="animate-spin" /> : <Plus size={14} />}
                    Create Jaz GitHub App
                  </Button>
                  <button
                    type="button"
                    onClick={() => setShowAdvancedManifest((open) => !open)}
                    className="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-[12px] text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink"
                  >
                    <ChevronDown
                      size={13}
                      className={`transition-transform duration-150 ${showAdvancedManifest ? '' : '-rotate-90'}`}
                    />
                    Options
                  </button>
                </div>

                <Collapse open={showAdvancedManifest}>
                  <div className="grid gap-3 pt-1 sm:grid-cols-2">
                    <label className="block">
                      <span className="mb-1 block text-[12px] font-medium text-ink-2">GitHub organization (optional)</span>
                      <Input
                        type="text"
                        value={githubOrg}
                        onChange={(e) => setGithubOrg(e.target.value)}
                        placeholder="your-org"
                      />
                      <span className="mt-1 block text-[11.5px] text-ink-3">
                        Leave empty to create under your personal GitHub account.
                      </span>
                    </label>
                    <label className="block">
                      <span className="mb-1 block text-[12px] font-medium text-ink-2">App name</span>
                      <Input
                        type="text"
                        value={appName}
                        onChange={(e) => setAppName(e.target.value)}
                        placeholder={DEFAULT_GITHUB_APP_NAME}
                      />
                      <span className="mt-1 block text-[11.5px] text-ink-3">
                        Defaults to {DEFAULT_GITHUB_APP_NAME}. Change only if already taken on GitHub.
                      </span>
                    </label>
                  </div>
                </Collapse>
              </div>
            ) : null}
          </SettingsCard>

          <SettingsCard className="p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="min-w-0 flex-1">
                <h2 className="text-[14px] font-semibold text-ink">Your GitHub account</h2>
                <p className="mt-0.5 text-[12.5px] text-ink-3">
                  {status.user_linked
                    ? status.github_login
                      ? `Connected as @${status.github_login}. Used to verify repository access and attribute pull requests.`
                      : 'Connected. Used to verify repository access and attribute pull requests.'
                    : 'Link your personal GitHub account to browse repositories and create pull requests under your identity.'}
                </p>
              </div>
              {status.user_linked ? (
                <div className="flex items-center gap-2">
                  <span className="inline-flex items-center gap-1 text-[12px] font-medium text-ok">
                    <CheckCircle2 size={14} />
                    Connected
                  </span>
                  {!isViewer ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={disconnectAccount.isPending}
                      onClick={() => disconnectAccount.mutate()}
                    >
                      {disconnectAccount.isPending ? <LoaderCircle size={13} className="animate-spin" /> : null}
                      Disconnect
                    </Button>
                  ) : null}
                </div>
              ) : !isViewer ? (
                <Button
                  variant="primary"
                  size="sm"
                  disabled={!status.configured || oauth.isPending}
                  onClick={() => oauth.mutate(undefined)}
                >
                  {oauth.isPending ? <LoaderCircle size={13} className="animate-spin" /> : <LogIn size={13} />}
                  Connect GitHub
                </Button>
              ) : null}
            </div>
          </SettingsCard>

          {status.configured || status.installations.length > 0 ? (
            <div className="flex flex-col gap-2.5">
              <div className="flex items-center justify-between">
                <h2 className="text-[13px] font-semibold text-ink-2">Installations</h2>
                <span className="text-[12px] text-ink-3">
                  {status.installations.length} {status.installations.length === 1 ? 'installation' : 'installations'}
                </span>
              </div>
              {status.installations.length === 0 ? (
                <SettingsCard className="px-4 py-3 text-[12.5px] text-ink-3">
                  No GitHub App installations yet. Install the app on your organization or personal account above.
                </SettingsCard>
              ) : (
                status.installations.map((installation) => {
                  const isOwnPersonal =
                    installation.scope === 'user' &&
                    (!installation.installed_by_user_id || installation.installed_by_user_id === cloud.user?.id)
                  const canManage = !isViewer && (installation.scope === 'org' ? isAdmin : isOwnPersonal)
                  return (
                    <InstallationRow
                      key={installation.installation_id}
                      installation={installation}
                      canManage={canManage}
                    />
                  )
                })
              )}
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function InstallationRow({
  installation,
  canManage,
}: {
  installation: CloudGitHubInstallation
  canManage: boolean
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [expanded, setExpanded] = useState(false)
  const [allowedReposText, setAllowedReposText] = useState(() => installation.allowed_repos.join('\n'))
  const [protectedRefsText, setProtectedRefsText] = useState(() => installation.protected_refs.join('\n'))
  const [validationError, setValidationError] = useState('')
  const [confirmRemove, setConfirmRemove] = useState(false)

  useEffect(() => {
    setAllowedReposText(installation.allowed_repos.join('\n'))
    setProtectedRefsText(installation.protected_refs.join('\n'))
    setValidationError('')
  }, [installation.installation_id, installation.allowed_repos, installation.protected_refs])

  const savePolicy = useMutation({
    mutationFn: async () => {
      const allowedRepos = parseLineList(allowedReposText)
      for (const repo of allowedRepos) {
        if (!splitRepoFullName(repo)) {
          throw new Error(`Invalid repository "${repo}" (expected owner/repo)`)
        }
      }
      const protectedRefs = parseLineList(protectedRefsText)
      for (const ref of protectedRefs) {
        if (!ref.startsWith('refs/') || /[\s\0]/.test(ref)) {
          throw new Error(`Protected ref "${ref}" must start with refs/`)
        }
      }
      return patchCloudGitHubInstallation(installation.installation_id, {
        allowed_repos: allowedRepos,
        protected_refs: protectedRefs,
      })
    },
    onMutate: () => setValidationError(''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
      toast(`Updated policy for ${installation.account_login}`)
    },
    onError: (err: Error) => {
      setValidationError(err.message)
      toast(err.message || 'Could not save installation policy', 'danger')
    },
  })

  const remove = useMutation({
    mutationFn: () => deleteCloudGitHubInstallation(installation.installation_id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.cloudGitHubRoot })
      toast(`Removed ${installation.account_login} installation`)
    },
    onError: (err: Error) => {
      toast(err.message || 'Could not remove installation', 'danger')
    },
  })

  const scopeLabel = installation.scope === 'user' ? 'Personal' : 'Organization'
  const selectionLabel =
    installation.repository_selection === 'selected' ? 'Selected repositories' : 'All repositories'

  return (
    <SettingsCard className="overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        aria-expanded={expanded}
        className="group flex w-full cursor-pointer items-center gap-2.5 px-3.5 py-2.5 text-left transition-colors duration-150 hover:bg-surface-2/50"
      >
        <ChevronDown
          size={14}
          className={`shrink-0 text-ink-3 transition-transform duration-150 group-hover:text-ink ${expanded ? '' : '-rotate-90'}`}
        />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="truncate text-[13.5px] font-medium text-ink">{installation.account_login}</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[11px] text-ink-2">{scopeLabel}</span>
            {installation.suspended_at ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-danger/12 px-2 py-0.5 text-[11px] text-danger">
                <CircleAlert size={11} />
                Suspended
              </span>
            ) : null}
          </span>
          <span className="mt-0.5 block text-[12px] text-ink-3">
            {selectionLabel}
            {installation.allowed_repos.length > 0
              ? ` · ${installation.allowed_repos.length} allowlisted`
              : ''}
            {installation.protected_refs.length > 0
              ? ` · ${installation.protected_refs.length} protected ${installation.protected_refs.length === 1 ? 'ref' : 'refs'}`
              : ''}
          </span>
        </span>
      </button>

      <Collapse open={expanded}>
        <div className="flex flex-col gap-3 border-t border-border/70 px-3.5 py-3">
          <label className="block">
            <span className="mb-1 block text-[12px] font-medium text-ink-2">Allowed repositories (optional)</span>
            <textarea
              rows={2}
              value={allowedReposText}
              disabled={!canManage || savePolicy.isPending}
              onChange={(e) => setAllowedReposText(e.target.value)}
              placeholder="owner/repo (one per line; empty allows all repos granted on GitHub)"
              className="w-full rounded-control border border-border bg-bg px-2.5 py-1.5 font-mono text-[12px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:opacity-60"
            />
          </label>

          <label className="block">
            <span className="mb-1 block text-[12px] font-medium text-ink-2">Protected refs (optional)</span>
            <textarea
              rows={2}
              value={protectedRefsText}
              disabled={!canManage || savePolicy.isPending}
              onChange={(e) => setProtectedRefsText(e.target.value)}
              placeholder="refs/heads/release/* (empty by default so GitHub branch protections govern)"
              className="w-full rounded-control border border-border bg-bg px-2.5 py-1.5 font-mono text-[12px] text-ink placeholder:text-ink-3 focus:border-primary focus:outline-none disabled:opacity-60"
            />
            <span className="mt-1 block text-[11.5px] text-ink-3">
              Leave empty to rely on GitHub&apos;s branch protections. Default branches and tags remain protected automatically.
            </span>
          </label>

          {validationError ? (
            <p role="alert" className="text-[12px] text-danger">
              {validationError}
            </p>
          ) : null}

          {canManage ? (
            <div className="flex flex-wrap items-center justify-between gap-2 pt-0.5">
              {confirmRemove ? (
                <div className="flex items-center gap-1.5">
                  <Button
                    variant="danger"
                    size="sm"
                    disabled={remove.isPending}
                    onClick={() => remove.mutate()}
                  >
                    {remove.isPending ? <LoaderCircle size={13} className="animate-spin" /> : <Trash2 size={13} />}
                    Confirm remove
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setConfirmRemove(false)}>
                    Cancel
                  </Button>
                </div>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-danger hover:text-danger"
                  disabled={savePolicy.isPending}
                  onClick={() => setConfirmRemove(true)}
                >
                  <Trash2 size={13} />
                  Remove
                </Button>
              )}

              <Button
                variant="primary"
                size="sm"
                disabled={savePolicy.isPending || remove.isPending}
                onClick={() => savePolicy.mutate()}
              >
                {savePolicy.isPending ? <LoaderCircle size={13} className="animate-spin" /> : null}
                Save policy
              </Button>
            </div>
          ) : null}
        </div>
      </Collapse>
    </SettingsCard>
  )
}
