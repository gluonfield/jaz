import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useToast } from '@/components/ui/toast'
import {
  closeConnectionQR,
  connectionQRStatus,
  startConnectionPlugin,
} from '@/lib/api/connections'
import type { ConnectionQRStart, IntegrationPlugin } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

type ActiveQR = {
  plugin: IntegrationPlugin
  qr: ConnectionQRStart
}

type ConnectRequest = {
  pluginID: string
  replacingSessionID?: string
}

export function useConnectionSignIn({
  plugins,
  onOAuthURL,
  onStartAccepted,
}: {
  plugins: IntegrationPlugin[]
  onOAuthURL: (url: string) => void
  onStartAccepted: () => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [activeQR, setActiveQRState] = useState<ActiveQR | null>(null)
  const activeQRRef = useRef<ActiveQR | null>(null)

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
        onStartAccepted()
        onOAuthURL(result.auth_url)
        toast('Finish sign-in in your browser')
        return
      }
      if (result.type === 'qr' && result.qr) {
        handleQRStart(result.qr, request)
        return
      }
      toast("Connection didn't return a usable sign-in method", 'danger')
    },
    onError: (error: Error, request) => {
      const plugin = plugins.find((item) => item.id === request.pluginID)
      if (plugin?.auth[0]?.kind === 'session') {
        toast(qrSignInError(error), 'danger')
        return
      }
      toast(`Couldn't start sign-in: ${error.message}`, 'danger')
    },
  })

  const handleQRStart = (qr: ConnectionQRStart, request: ConnectRequest) => {
    if (request.replacingSessionID && activeQRRef.current?.qr.session_id !== request.replacingSessionID) {
      void closeConnectionQR(qr.session_id).catch(() => undefined)
      return
    }
    const plugin =
      plugins.find((item) => item.id === (qr.provider || request.pluginID)) ?? activeQRRef.current?.plugin
    if (!plugin) {
      void closeConnectionQR(qr.session_id).catch(() => undefined)
      toast("Connection didn't return a usable sign-in method", 'danger')
      return
    }
    onStartAccepted()
    setActiveQR({ plugin, qr })
    if (request.replacingSessionID && request.replacingSessionID !== qr.session_id) {
      queryClient.removeQueries({ queryKey: keys.connectionQR(request.replacingSessionID) })
      void closeConnectionQR(request.replacingSessionID).catch(() => undefined)
    }
    toast(`Scan the ${plugin.name} QR code`)
  }

  const start = (plugin: IntegrationPlugin) => {
    if (plugin.implementation.status !== 'available') return
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
    activeQR,
    connectingPluginID: connect.variables?.pluginID,
    isConnecting: connect.isPending,
    qrStatus: qrStatus.data,
    qrLoading: qrStatus.isFetching,
    qrRefreshing:
      connect.isPending &&
      connect.variables?.pluginID === activeQR?.plugin.id &&
      connect.variables?.replacingSessionID === activeQR?.qr.session_id,
    closeQR,
    refreshQR,
    start,
  }
}

function closeQRSession(queryClient: QueryClient, sessionID?: string) {
  if (!sessionID) return
  queryClient.removeQueries({ queryKey: keys.connectionQR(sessionID) })
  void closeConnectionQR(sessionID).catch(() => undefined)
}

function qrSignInError(error: Error): string {
  if (error.message.toLowerCase().includes('timed out')) {
    return "Couldn't show the QR code. Try again."
  }
  return "Couldn't start QR sign-in. Try again."
}
