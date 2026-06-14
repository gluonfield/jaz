import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Bot, CheckCircle2, Copy, KeyRound, LoaderCircle, RefreshCw, Server } from 'lucide-react'
import { motion } from 'motion/react'
import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Select } from '@/components/ui/Select'
import { SkeletonRows } from '@/components/ui/Skeleton'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/toast'
import { agentLabel } from '@/lib/agentLabel'
import { completeOnboarding, onboardingQuery } from '@/lib/api/onboarding'
import type { ACPAgentAuth, AgentSettings, OnboardingACPProbe, OnboardingStatus } from '@/lib/api/types'
import { writeClipboard } from '@/lib/clipboard'
import { keys } from '@/lib/query/keys'

const EASE = [0.22, 1, 0.36, 1] as const

export function OnboardingGate({ children }: { children: ReactNode }) {
  const onboarding = useQuery(onboardingQuery)

  if (window.jaz?.windowKind === 'board') return <>{children}</>
  if (onboarding.isPending) return <OnboardingShell><SkeletonRows count={4} /></OnboardingShell>
  if (onboarding.isError) {
    return (
      <OnboardingShell>
        <StatusBlock
          icon={<AlertCircle size={16} />}
          title="Couldn't load onboarding"
          text={onboarding.error.message}
        />
      </OnboardingShell>
    )
  }
  if (onboarding.data.completed) return <>{children}</>
  return <OnboardingScreen status={onboarding.data} onRefresh={() => void onboarding.refetch()} />
}

function OnboardingScreen({ status, onRefresh }: { status: OnboardingStatus; onRefresh: () => void }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState(() => draftFromStatus(status))
  const [keysByProvider, setKeysByProvider] = useState<Record<string, string>>({})
  const [acpKeysByAgent, setACPKeysByAgent] = useState<Record<string, string>>({})

  useEffect(() => {
    setDraft(draftFromStatus(status))
  }, [status])

  const providerStatus = useMemo(
    () => new Map(status.native_providers.map((provider) => [provider.id, provider])),
    [status.native_providers],
  )
  const selectedProvider = draft.native.model_provider || draft.providers[0]?.id || ''
  const selectedProviderStatus = providerStatus.get(selectedProvider)
  const selectedProviderKey = keysByProvider[selectedProvider]?.trim() ?? ''
  const acpStatus = useMemo(() => new Map(status.acp.map((probe) => [probe.agent, probe])), [status.acp])
  const acpEnabled = draft.agents.some((agent) => {
    if (!draft.acp[agent]?.enabled) return false
    const probe = acpStatus.get(agent)
    return Boolean(probe?.available || acpKeysByAgent[agent]?.trim())
  })
  const nativeReady = Boolean(selectedProviderStatus?.configured || selectedProviderKey)
  const canFinish = acpEnabled || nativeReady

  const copyAuthCommand = useCallback(async (command: string) => {
    if (await writeClipboard(command)) {
      toast('Copied sign-in command')
    } else {
      toast("Couldn't copy sign-in command", 'danger')
    }
  }, [toast])

  const save = useMutation({
    mutationFn: () =>
      completeOnboarding({
        settings: draft,
        provider_keys: selectedProviderKey ? { [selectedProvider]: selectedProviderKey } : undefined,
        acp_keys: compactSecrets(acpKeysByAgent),
        completed: true,
      }),
    onSuccess: (saved) => {
      queryClient.setQueryData(keys.onboarding, saved)
      queryClient.invalidateQueries({ queryKey: keys.agentSettings })
      queryClient.invalidateQueries({ queryKey: keys.acpAgents })
    },
  })

  const setProvider = (model_provider: string) => {
    const next = draft.providers.find((provider) => provider.id === model_provider)
    const current = draft.providers.find((provider) => provider.id === draft.native.model_provider)
    const model =
      draft.native.model.trim() === '' || draft.native.model === current?.default_model
        ? next?.default_model || draft.native.model
        : draft.native.model
    setDraft({ ...draft, native: { ...draft.native, model_provider, model } })
  }

  return (
    <OnboardingShell>
      <motion.div
        initial={{ opacity: 0, y: 10, filter: 'blur(6px)' }}
        animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
        transition={{ duration: 0.45, ease: EASE }}
        className="min-w-0 w-full max-w-[calc(100vw-40px)] md:max-w-[640px]"
      >
        <div className="mb-5">
          <h1 className="text-balance text-[22px] font-semibold text-ink">Connect Jaz to its agents</h1>
          <p className="mt-2 max-w-[620px] text-pretty text-[13px] text-ink-3">
            Agent credentials are checked on the backend machine. A remote backend needs its own
            Codex, Claude, and Grok sign-ins.
          </p>
        </div>

        <div className="overflow-hidden rounded-[14px] bg-surface/85 p-1 shadow-[0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent),0_18px_60px_rgba(0,0,0,0.10)] backdrop-blur-[2px]">
          <StepRow icon={<Server size={16} />} title="Backend" detail="Connected">
            <CheckCircle2 size={17} className="text-primary" />
          </StepRow>

          <div className="border-t border-border/70 px-3 py-3">
            <div className="mb-3 flex items-center gap-2">
              <Bot size={16} className="text-ink-3" />
              <p className="text-[13px] font-medium text-ink">ACP clients</p>
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={onRefresh}
                aria-label="Refresh agent status"
                title="Refresh agent status"
              >
                <RefreshCw size={13} />
                Refresh
              </Button>
            </div>
            <div className="grid gap-2">
              {status.acp.map((probe) => (
                <AgentToggle
                  key={probe.agent}
                  probe={probe}
                  enabled={Boolean(draft.acp[probe.agent]?.enabled)}
                  apiKeyValue={acpKeysByAgent[probe.agent] ?? ''}
                  onCopyAuthCommand={copyAuthCommand}
                  onAPIKeyChange={(value) =>
                    setACPKeysByAgent({ ...acpKeysByAgent, [probe.agent]: value })
                  }
                  onChange={(enabled) =>
                    setDraft({
                      ...draft,
                      acp: {
                        ...draft.acp,
                        [probe.agent]: { ...draft.acp[probe.agent], enabled },
                      },
                    })
                  }
                />
              ))}
            </div>
          </div>

          <div className="border-t border-border/70 px-3 py-3">
            <div className="mb-3 flex items-center gap-2">
              <KeyRound size={16} className="text-ink-3" />
              <p className="text-[13px] font-medium text-ink">Native Agent</p>
            </div>
            <div className="grid gap-3 md:grid-cols-[220px_minmax(0,1fr)] md:items-start">
              <Select
                value={selectedProvider}
                options={draft.providers.map((provider) => ({
                  value: provider.id,
                  label: provider.label,
                  description: provider.api_key_env,
                }))}
                onChange={setProvider}
                disabled={save.isPending}
                aria-label="Native provider"
                className="h-9"
              />
              {selectedProviderStatus?.configured ? (
                <div className="flex min-h-9 items-center rounded-control bg-primary/10 px-3 text-[13px] text-ink">
                  {selectedProviderStatus.api_key_env} configured
                </div>
              ) : (
                <Input
                  type="password"
                  value={keysByProvider[selectedProvider] ?? ''}
                  onChange={(event) =>
                    setKeysByProvider({ ...keysByProvider, [selectedProvider]: event.target.value })
                  }
                  disabled={save.isPending || !selectedProvider}
                  placeholder={selectedProviderStatus?.api_key_env || 'API key'}
                  autoComplete="off"
                  spellCheck={false}
                />
              )}
            </div>
          </div>
        </div>

        <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="min-h-5 text-[12px] text-ink-3">
            {!canFinish ? 'Sign in to an ACP client on this backend or add a Native Agent key.' : save.error?.message}
          </p>
          <Button
            variant="primary"
            size="lg"
            disabled={!canFinish || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending && <LoaderCircle size={14} className="animate-spin" />}
            Finish setup
          </Button>
        </div>
      </motion.div>
    </OnboardingShell>
  )
}

function AgentToggle({
  probe,
  enabled,
  apiKeyValue,
  onCopyAuthCommand,
  onAPIKeyChange,
  onChange,
}: {
  probe: OnboardingACPProbe
  enabled: boolean
  apiKeyValue: string
  onCopyAuthCommand: (command: string) => void
  onAPIKeyChange: (value: string) => void
  onChange: (enabled: boolean) => void
}) {
  const status = agentStatusText(probe)
  const canCopyAuth = Boolean(probe.auth_command && probe.auth_command_available)
  const profile = authProfileText(probe)
  const apiKeyEnv = probe.api_key?.source_env
  const apiKeyReady = Boolean(probe.api_key_configured || apiKeyValue.trim())
  return (
    <div className="rounded-[12px] bg-bg p-2.5 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <p className="truncate text-[13px] font-medium text-ink">{agentLabel(probe.agent)}</p>
            <span className={`text-[12px] ${probe.available ? 'text-primary' : 'text-ink-3'}`}>{status}</span>
          </div>
          <p className={`mt-1 break-words text-pretty text-[12px] ${probe.available ? 'text-ink-3' : 'text-danger'}`}>
            {probe.reason || authStorageText(probe)}
          </p>
          <p className="mt-1 text-pretty text-[12px] text-ink-3">{profile}</p>
          {probe.storage_path && (
            <p className="mt-1 truncate font-mono text-[11px] text-ink-3">{probe.storage_path}</p>
          )}
          {apiKeyEnv ? (
            <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <Input
                type="password"
                value={apiKeyValue}
                onChange={(event) => onAPIKeyChange(event.target.value)}
                placeholder={probe.api_key_configured ? `${apiKeyEnv} configured` : apiKeyEnv}
                autoComplete="off"
                spellCheck={false}
                className="h-8 rounded-full bg-surface px-3 py-0 text-[12px]"
                aria-label={`${agentLabel(probe.agent)} API key fallback`}
              />
              <span
                className={`inline-flex h-8 items-center justify-center rounded-full px-2.5 text-[12px] ${
                  apiKeyReady ? 'bg-primary-soft text-primary-strong' : 'bg-surface text-ink-3'
                }`}
              >
                {apiKeyReady ? 'API key ready' : 'Fallback'}
              </span>
            </div>
          ) : null}
          {!probe.authenticated && probe.auth_command && (
            <div className="mt-2 grid gap-2 rounded-[calc(var(--radius-control)-2px)] bg-surface px-2.5 py-2">
              <p className="text-[11px] text-ink-3">Run this on the backend host, then refresh.</p>
              <code className="overflow-hidden text-ellipsis whitespace-nowrap font-mono text-[11px] text-ink-2">
                {probe.auth_command}
              </code>
              {!probe.auth_command_available && probe.auth_command_reason && (
                <p className="text-[11px] text-danger">{probe.auth_command_reason}</p>
              )}
            </div>
          )}
        </div>
        <div className="flex items-center justify-between gap-2 sm:justify-end">
          {!probe.authenticated && probe.auth_command && (
            <Button
              variant="secondary"
              size="sm"
              disabled={!canCopyAuth}
              onClick={() => onCopyAuthCommand(probe.auth_command || '')}
            >
              <Copy size={13} />
              Copy
            </Button>
          )}
          <Switch
            checked={enabled}
            disabled={!probe.installed || (!probe.available && !apiKeyReady)}
            onChange={onChange}
            aria-label={`Enable ${agentLabel(probe.agent)}`}
          />
        </div>
      </div>
    </div>
  )
}

function agentStatusText(probe: OnboardingACPProbe): string {
  if (!probe.installed) return 'Missing'
  if (!probe.authenticated) return 'Needs sign-in'
  if (!probe.available) return 'Needs setup'
  return 'Ready'
}

function authStorageText(probe: OnboardingACPProbe): string {
  if (probe.auth_kind === 'api_key') return 'Using explicit API key fallback.'
  if (probe.refresh_owner === 'coding_agent_cli') return 'The coding agent owns token refresh.'
  return 'Detected'
}

function authProfileText(probe: OnboardingACPProbe): string {
  if (probe.auth_kind === 'api_key') return `${probe.api_key?.source_env || 'API key'} configured.`
  const label = probe.auth_source === 'existing_cli' ? 'Using existing CLI profile' : 'Using Jaz profile'
  const evidence =
    probe.auth_evidence === 'keyring_config'
      ? 'keychain configured'
      : probe.auth_evidence === 'auth_json'
        ? 'auth.json found'
        : probe.auth_evidence === 'credentials_json'
          ? 'credentials file found'
          : probe.auth_evidence === 'env'
            ? 'environment credential found'
            : ''
  return evidence ? `${label}; ${evidence}.` : `${label}.`
}

function StepRow({ icon, title, detail, children }: { icon: ReactNode; title: string; detail: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-3 py-3">
      <span className="grid size-8 place-items-center rounded-full bg-bg text-ink-3 shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-border)_70%,transparent)]">{icon}</span>
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-[12px] text-ink-3">{detail}</p>
      </div>
      {children}
    </div>
  )
}

function StatusBlock({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className="mx-auto flex w-full max-w-[420px] items-start gap-3 rounded-card bg-surface p-4 text-ink">
      <span className="mt-0.5 text-danger">{icon}</span>
      <div>
        <p className="text-[13px] font-medium">{title}</p>
        <p className="mt-1 text-[12px] text-ink-3">{text}</p>
      </div>
    </div>
  )
}

function OnboardingShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full flex-col bg-bg">
      <div className="titlebar-drag h-[52px] shrink-0" />
      <main className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-5 pb-[52px]">
        <div className="flex min-h-full w-full items-start justify-center py-6 md:py-8">
          {children}
        </div>
      </main>
    </div>
  )
}

function draftFromStatus(status: OnboardingStatus): AgentSettings {
  const settings = cloneSettings(status.settings)
  for (const probe of status.acp) {
    const current = settings.acp[probe.agent]
    settings.acp[probe.agent] = {
      ...current,
      auth: onboardingAuth(current?.auth, probe.recommended_auth),
      enabled: probe.available && Boolean(settings.acp[probe.agent]?.enabled ?? true),
    }
  }
  return settings
}

function onboardingAuth(current?: ACPAgentAuth, recommended?: ACPAgentAuth): ACPAgentAuth {
  if (current?.mode && current.mode !== 'auto') return current
  return {
    mode: recommended?.mode || current?.mode || 'auto',
    path: recommended?.path ?? current?.path ?? '',
  }
}

function cloneSettings(settings: AgentSettings): AgentSettings {
  return {
    native: { ...settings.native },
    providers: [...(settings.providers ?? [])],
    acp_auth: { ...(settings.acp_auth ?? {}) },
    acp_keys: { ...(settings.acp_keys ?? {}) },
    acp: Object.fromEntries(
      Object.entries(settings.acp ?? {}).map(([agent, value]) => [
        agent,
        { ...value, auth: value.auth ? { ...value.auth } : undefined },
      ]),
    ),
    agents: [...(settings.agents ?? [])],
  }
}

function compactSecrets(values: Record<string, string>): Record<string, string> | undefined {
  const out = Object.fromEntries(
    Object.entries(values)
      .map(([key, value]) => [key, value.trim()] as const)
      .filter(([, value]) => value.length > 0),
  )
  return Object.keys(out).length > 0 ? out : undefined
}
