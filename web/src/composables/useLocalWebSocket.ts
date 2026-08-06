import { ref, onMounted, onUnmounted } from 'vue'

// ────────────────────────────────────────────────────
// Singleton WebSocket connection (module level, shared across components)
// ────────────────────────────────────────────────────

type MessageHandler = (data: any) => void

let socket: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
const handlers = new Map<string, Set<MessageHandler>>()
let intentionalClose = false

function wsUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/local/ws`
}

function connect() {
  if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return
  intentionalClose = false
  socket = new WebSocket(wsUrl())

  socket.onopen = () => {
    // No additional operations are required after reconnection. The backend will send a welcome message (if any) when the connection is established.
  }

  socket.onmessage = (raw: MessageEvent<string>) => {
    let payload: Record<string, any>
    try {
      payload = JSON.parse(raw.data)
    } catch {
      return
    }
    const type = payload.type as string
    if (!type) return
    const set = handlers.get(type)
    if (set) {
      for (const fn of set) {
        try {
          fn(payload.data ?? payload)
        } catch {
          // Business handler exception does not affect the connection
        }
      }
    }
  }

  socket.onclose = () => {
    socket = null
    if (!intentionalClose) {
      reconnectTimer = setTimeout(connect, 2000)
    }
  }

  socket.onerror = () => {
    //onclose will be triggered immediately, no need to reconnect here
  }
}

function disconnect() {
  intentionalClose = true
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  socket?.close()
  socket = null
}

/** * Sends a JSON message to the current WS connection.  * Silently discard when the connection is not ready.  */
export function wsSend(msg: Record<string, any>) {
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(msg))
  }
}

/** Register a processor of a certain message type and return the unregistration function */
export function onMessage(type: string, handler: MessageHandler): () => void {
  if (!handlers.has(type)) handlers.set(type, new Set())
  handlers.get(type)!.add(handler)

  // Make sure the connection is started
  connect()

  return () => {
    const set = handlers.get(type)
    if (set) {
      set.delete(handler)
      if (set.size === 0) handlers.delete(type)
    }
  }
}

/** Get the underlying socket (for advanced usage, such as TaskExecutionConsole's subscribe) */
export function getSocket(): WebSocket | null {
  return socket
}

/** Whether currently connected */
export function isConnected(): boolean {
  return socket?.readyState === WebSocket.OPEN
}

// ────────────────────────────────────────────────────
// Vue combined function
// ────────────────────────────────────────────────────

/** * Listen to local WebSocket push messages in the component.  * Automatically establish a connection when the component is mounted (if not connected), and automatically cancel monitoring when uninstalled.  * * @example * useLocalWS('app.cli_count', (data) => { * appStore.setCliTaskCount(data.count) * }) */
export function useLocalWS(type: string, handler: MessageHandler) {
  let unsubscribe: (() => void) | null = null

  onMounted(() => {
    unsubscribe = onMessage(type, handler)
  })

  onUnmounted(() => {
    unsubscribe?.()
    unsubscribe = null
  })
}

/** * Returns a ref<boolean>, reflecting the current WebSocket connection status.  */
export function useLocalWSStatus() {
  const connected = ref(isConnected())
  let timer: ReturnType<typeof setInterval> | null = null

  onMounted(() => {
    connect()
    timer = setInterval(() => {
      connected.value = isConnected()
    }, 1000)
  })

  onUnmounted(() => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  })

  return connected
}

/** Actively disconnect the singleton connection (called when logging out/account switching) */
export { disconnect as disconnectLocalWS }
