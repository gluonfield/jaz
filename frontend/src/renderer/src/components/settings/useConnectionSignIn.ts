import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  closeConnectionQR,
  connectionPluginsQuery,
  connectionQRStatus,
  startConnectionPlugin,
  submitConnectionQRPassword,
} from '@/lib/api/connections'
import { clientRuntime } from '@/lib/clientRuntime'
import type { ConnectionQRStart, IntegrationPlugin } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'
import { pluginCanConnect } from './connectionFormatting'

type ActiveQR = {
  plugin: IntegrationPlugin
  qr: ConnectionQRStart
}

type ConnectRequest = {
  pluginID: string
  replacingSessionID?: string
}

// Owns the whole first-party connect flow: the plugin catalog (with a fast
// poll window after an OAuth hand-off to the browser), OAuth URL opening, and
// the QR session lifecycle.
export function useConnectionSignIn({ onStartAccepted }: { onStartAccepted?: () => void } = {}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [pollUntil, setPollUntil] = useState(0)
  const [activeQR, setActiveQRState] = useState<ActiveQR | null>(null)
  const activeQRRef = useRef<ActiveQR | null>(null)

  const plugins = useQuery({
    ...connectionPluginsQuery,
    refetchInterval: () => (Date.now() < pollUntil ? 2000 : false),
  })
  const pluginList = plugins.data ?? []

  const setActiveQR = useCallback((next: ActiveQR | null) => {
    activeQRRef.current = next
    setActiveQRState(next)
  }, [])

  const qrStatus = useQuery({
    queryKey: keys.connectionQR(activeQR?.qr.session_id ?? ''),
    queryFn: () => connectionQRStatus(activeQR?.qr.session_id ?? ''),
    enabled: Boolean(activeQR),
    refetchInterval: (query) => {
      const status = query.state.data?.status ?? activeQR?.qr.status
      return status === 'pending' || status === 'scanned' ? 2000 : false
    },
  })

  const connect = useMutation({
    mutationFn: (request: ConnectRequest) => startConnectionPlugin(request.pluginID),
    onSuccess: (result, request) => {
      if (result.type === 'oauth' && result.auth_url) {
        onStartAccepted?.()
        setPollUntil(Date.now() + 90_000)
        openAuthURL(result.auth_url)
        toast('Finish sign-in in your browser')
        return
      }
      if (result.type === 'qr' && result.qr) {
        handleQRStart(result.qr, request)
        return
      }
      if (result.type === 'mcp' && result.mcp) {
        onStartAccepted?.()
        void queryClient.invalidateQueries({ queryKey: keys.connectionPlugins })
        void queryClient.invalidateQueries({ queryKey: keys.mcpServers })
        toast(`Added ${result.mcp.name} MCP server`)
        return
      }
      toast("Connection didn't return a usable sign-in method", 'danger')
    },
    onError: (error: Error, request) => {
      const plugin = pluginList.find((item) => item.id === request.pluginID)
      if (plugin?.auth[0]?.kind === 'session') {
        toast(qrSignInError(error, plugin.name), 'danger')
        return
      }
      toast(`Couldn't start sign-in: ${error.message}`, 'danger')
    },
  })

  const qrPassword = useMutation({
    mutationFn: (request: { sessionID: string; password: string }) =>
      submitConnectionQRPassword(request.sessionID, request.password),
    onSuccess: (status) => {
      queryClient.setQueryData(keys.connectionQR(status.session_id), status)
    },
    onError: (error: Error) => {
      toast(error.message || "Couldn't submit QR password", 'danger')
    },
  })

  const handleQRStart = (qr: ConnectionQRStart, request: ConnectRequest) => {
    if (request.replacingSessionID && activeQRRef.current?.qr.session_id !== request.replacingSessionID) {
      void closeConnectionQR(qr.session_id).catch(() => undefined)
      return
    }
    const plugin =
      pluginList.find((item) => item.id === (qr.provider || request.pluginID)) ?? activeQRRef.current?.plugin
    if (!plugin) {
      void closeConnectionQR(qr.session_id).catch(() => undefined)
      toast("Connection didn't return a usable sign-in method", 'danger')
      return
    }
    onStartAccepted?.()
    setActiveQR({ plugin, qr })
    if (request.replacingSessionID && request.replacingSessionID !== qr.session_id) {
      queryClient.removeQueries({ queryKey: keys.connectionQR(request.replacingSessionID) })
      void closeConnectionQR(request.replacingSessionID).catch(() => undefined)
    }
    toast(`Scan the ${plugin.name} QR code`)
  }

  const start = (plugin: IntegrationPlugin) => {
    if (!pluginCanConnect(plugin)) return
    connect.mutate({ pluginID: plugin.id })
  }

  const closeQR = () => {
    const sessionID = activeQRRef.current?.qr.session_id
    setActiveQR(null)
    closeQRSession(queryClient, sessionID)
  }

  const refreshQR = () => {
    const current = activeQRRef.current
    if (!current || connect.isPending) return
    connect.mutate({ pluginID: current.plugin.id, replacingSessionID: current.qr.session_id })
  }

  const submitQRPassword = (password: string) => {
    const current = activeQRRef.current
    if (!current || qrPassword.isPending) return
    qrPassword.mutate({ sessionID: current.qr.session_id, password })
  }

  // While the OAuth poll window is open, a focus flip back from the browser
  // refreshes immediately instead of waiting out the interval.
  useEffect(() => {
    if (pollUntil === 0) return
    const refresh = () => {
      if (document.visibilityState === 'hidden') return
      void queryClient.invalidateQueries({ queryKey: keys.connectionPlugins })
    }
    const timeout = window.setTimeout(() => setPollUntil(0), Math.max(0, pollUntil - Date.now()))
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      window.clearTimeout(timeout)
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [pollUntil, queryClient])

  useEffect(() => {
    if (qrStatus.data?.status === 'connected') {
      void queryClient.invalidateQueries({ queryKey: keys.connectionPlugins })
    }
  }, [qrStatus.data?.status, queryClient])

  useEffect(() => {
    return () => {
      closeQRSession(queryClient, activeQRRef.current?.qr.session_id)
      activeQRRef.current = null
    }
  }, [queryClient])

  return {
    plugins,
    activeQR,
    connectingPluginID: connect.variables?.pluginID,
    isConnecting: connect.isPending,
    qrStatus: qrStatus.data,
    qrLoading: qrStatus.isFetching,
    qrRefreshing:
      connect.isPending &&
      connect.variables?.pluginID === activeQR?.plugin.id &&
      connect.variables?.replacingSessionID === activeQR?.qr.session_id,
    qrPasswordSubmitting: qrPassword.isPending,
    closeQR,
    refreshQR,
    submitQRPassword,
    start,
  }
}

function openAuthURL(url: string): void {
  if (clientRuntime.openExternalURL) {
    clientRuntime.openExternalURL(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

function closeQRSession(queryClient: QueryClient, sessionID?: string) {
  if (!sessionID) return
  queryClient.removeQueries({ queryKey: keys.connectionQR(sessionID) })
  void closeConnectionQR(sessionID).catch(() => undefined)
}

function qrSignInError(error: Error, provider: string): string {
  const message = error.message.trim()
  const lower = message.toLowerCase()
  if (lower.includes('qr provider unavailable')) {
    return `${provider} QR sign-in is not available in this build.`
  }
  if (lower.includes('timed out')) {
    return "Couldn't show the QR code. Try again."
  }
  return message || "Couldn't start QR sign-in. Try again."
}
