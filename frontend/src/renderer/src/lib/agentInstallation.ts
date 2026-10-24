import type { ACPAgentAuthStatus } from '@/lib/api/types'

export function agentInstallation(status?: ACPAgentAuthStatus) {
  const states = [status?.managed_adapter, status?.managed_tool]
  return states.find((item) => item?.state === 'failed' || item?.state === 'unsupported')
    ?? states.find((item) => item?.state === 'downloading')
    ?? states.find((item) => item?.state === 'missing')
}
