import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  closePluginQR,
  connectPlugin,
  pluginCatalogQuery,
  pluginQRStatus,
  submitPluginQRPassword,
  type ConnectionQRStart,
  type Plugin,
} from '@/lib/api/plugins'
import { openExternal } from '@/lib/clientRuntime'
import { keys } from '@/lib/query/keys'
import { pluginCanConnect } from './pluginFormatting'

type ActiveQR = {
  plugin: Plugin
  qr: ConnectionQRStart
}

type ConnectRequest = {
  pluginID: string
  replacingSessionID?: string
}

export function usePluginSignIn({ onStartAccepted }: { onStartAccepted?: () => void } = {}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [pollUntil, setPollUntil] = useState(0)
  const [activeQR, setActiveQRState] = useState<ActiveQR | null>(null)
  const activeQRRef = useRef<ActiveQR | null>(null)

  const catalog = useQuery({
    ...pluginCatalogQuery,
    refetchInterval: () => (Date.now() < pollUntil ? 2000 : false),
  })
  const pluginList = catalog.data?.plugins ?? []

  const setActiveQR = useCallback((next: ActiveQR | null) => {
    activeQRRef.current = next
    setActiveQRState(next)
  }, [])

  const qrStatus = useQuery({
    queryKey: keys.pluginQR(activeQR?.qr.session_id ?? ''),
    queryFn: () => pluginQRStatus(activeQR?.qr.session_id ?? ''),
    enabled: Boolean(activeQR),
    refetchInterval: (query) => {
      const status = query.state.data?.status ?? activeQR?.qr.status
      return status === 'pending' || status === 'scanned' ? 2000 : false
    },
  })

  const connect = useMutation({
    mutationFn: (request: ConnectRequest) => connectPlugin(request.pluginID),
    onSuccess: (result, request) => {
      if (result.type === 'oauth' && result.auth_url) {
        onStartAccepted?.()
        void queryClient.invalidateQueries({ queryKey: keys.plugins })
        setPollUntil(Date.now() + 90_000)
        openExternal(result.auth_url)
        toast('Finish sign-in in your browser')
        return
      }
      if (result.type === 'qr' && result.qr) {
        handleQRStart(result.qr, request)
        return
      }
      toast("Plugin didn't return a usable sign-in method", 'danger')
    },
    onError: (error: Error, request) => {
      const plugin = pluginList.find((item) => item.id === request.pluginID)
      if (plugin?.auth?.kind === 'session') {
        toast(qrSignInError(error, plugin.display_name), 'danger')
        return
      }
      toast(`Couldn't start sign-in: ${error.message}`, 'danger')
    },
  })

  const qrPassword = useMutation({
    mutationFn: (request: { sessionID: string; password: string }) =>
      submitPluginQRPassword(request.sessionID, request.password),
    onSuccess: (status) => {
      queryClient.setQueryData(keys.pluginQR(status.session_id), status)
    },
    onError: (error: Error) => {
      toast(error.message || "Couldn't submit QR password", 'danger')
    },
  })

  const handleQRStart = (qr: ConnectionQRStart, request: ConnectRequest) => {
    if (request.replacingSessionID && activeQRRef.current?.qr.session_id !== request.replacingSessionID) {
      void closePluginQR(qr.session_id).catch(() => undefined)
      return
    }
    const plugin =
      pluginList.find((item) => item.id === (qr.provider || request.pluginID)) ?? activeQRRef.current?.plugin
    if (!plugin) {
      void closePluginQR(qr.session_id).catch(() => undefined)
      toast("Plugin didn't return a usable sign-in method", 'danger')
      return
    }
    onStartAccepted?.()
    setActiveQR({ plugin, qr })
    if (request.replacingSessionID && request.replacingSessionID !== qr.session_id) {
      queryClient.removeQueries({ queryKey: keys.pluginQR(request.replacingSessionID) })
      void closePluginQR(request.replacingSessionID).catch(() => undefined)
    }
    toast(`Scan the ${plugin.display_name} QR code`)
  }

  const start = (plugin: Plugin) => {
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

  useEffect(() => {
    if (pollUntil === 0) return
    const refresh = () => {
      if (document.visibilityState === 'hidden') return
      void queryClient.invalidateQueries({ queryKey: keys.plugins })
      void queryClient.invalidateQueries({ queryKey: keys.mcpServers })
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
      void queryClient.invalidateQueries({ queryKey: keys.plugins })
    }
  }, [qrStatus.data?.status, queryClient])

  useEffect(() => {
    return () => {
      closeQRSession(queryClient, activeQRRef.current?.qr.session_id)
      activeQRRef.current = null
    }
  }, [queryClient])

  return {
    catalog,
    plugins: pluginList,
    connectingPluginID: connect.variables?.pluginID,
    isConnecting: connect.isPending,
    start,
    qrModal: {
      plugin: activeQR?.plugin,
      qr: activeQR?.qr,
      status: qrStatus.data,
      loading: qrStatus.isFetching,
      refreshing:
        connect.isPending &&
        connect.variables?.pluginID === activeQR?.plugin.id &&
        connect.variables?.replacingSessionID === activeQR?.qr.session_id,
      passwordSubmitting: qrPassword.isPending,
      onClose: closeQR,
      onRefresh: refreshQR,
      onSubmitPassword: submitQRPassword,
    },
  }
}

function closeQRSession(queryClient: QueryClient, sessionID?: string) {
  if (!sessionID) return
  queryClient.removeQueries({ queryKey: keys.pluginQR(sessionID) })
  void closePluginQR(sessionID).catch(() => undefined)
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
