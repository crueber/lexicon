// Reconnecting WebSocket client with event subscription system.

export type WSMessage = {
  type: string;
  payload: Record<string, unknown>;
};

export type WSHandler = (msg: WSMessage) => void;

const PING_INTERVAL_MS = 30_000;
const INITIAL_RECONNECT_DELAY_MS = 3_000;
const MAX_RECONNECT_DELAY_MS = 30_000;

export interface WSClient {
  /** Connect or reconnect to the WebSocket server. */
  connect(): void;
  /** Close the connection and stop reconnecting. */
  disconnect(): void;
  /** Subscribe to a specific message type. Returns an unsubscribe function. */
  on(type: string, handler: WSHandler): () => void;
  /** Subscribe to all messages. Returns an unsubscribe function. */
  onAny(handler: WSHandler): () => void;
}

export function createWebSocketClient(getToken: () => string | null): WSClient {
  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let pingTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectDelay = INITIAL_RECONNECT_DELAY_MS;
  let stopped = false;

  // Handlers keyed by message type.
  const handlers = new Map<string, Set<WSHandler>>();
  // Handlers that receive all messages.
  const anyHandlers = new Set<WSHandler>();

  function getWsUrl(): string {
    const token = getToken();
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const base = `${protocol}//${window.location.host}/ws`;
    return token ? `${base}?token=${encodeURIComponent(token)}` : base;
  }

  function dispatch(msg: WSMessage): void {
    // Dispatch to type-specific handlers.
    const typeHandlers = handlers.get(msg.type);
    if (typeHandlers) {
      for (const h of typeHandlers) {
        h(msg);
      }
    }
    // Dispatch to any-handlers.
    for (const h of anyHandlers) {
      h(msg);
    }
  }

  function startPing(): void {
    stopPing();
    pingTimer = setInterval(() => {
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "PING" }));
      }
    }, PING_INTERVAL_MS);
  }

  function stopPing(): void {
    if (pingTimer !== null) {
      clearInterval(pingTimer);
      pingTimer = null;
    }
  }

  function scheduleReconnect(): void {
    if (stopped) return;
    if (reconnectTimer !== null) return;

    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      if (!stopped) {
        connect();
      }
    }, reconnectDelay);

    // Exponential backoff, capped at max.
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY_MS);
  }

  function connect(): void {
    if (stopped) return;

    const token = getToken();
    if (!token) {
      // No token — don't connect yet.
      return;
    }

    // Close any existing connection.
    if (socket) {
      socket.onclose = null;
      socket.onerror = null;
      socket.onmessage = null;
      socket.close();
      socket = null;
    }

    const url = getWsUrl();
    socket = new WebSocket(url);

    socket.onopen = () => {
      // Reset backoff on successful connection.
      reconnectDelay = INITIAL_RECONNECT_DELAY_MS;
      startPing();
    };

    socket.onmessage = (event: MessageEvent) => {
      let msg: WSMessage;
      try {
        msg = JSON.parse(event.data as string) as WSMessage;
      } catch {
        return;
      }

      // Handle SESSION_REVOKED by clearing tokens and redirecting.
      if (msg.type === "SESSION_REVOKED") {
        disconnect();
        // Clear tokens from localStorage.
        localStorage.removeItem("lexicon_access_token");
        localStorage.removeItem("lexicon_refresh_token");
        window.location.href = "/login";
        return;
      }

      // Handle BOOK_UPDATED by dispatching a custom event for cache invalidation.
      if (msg.type === "BOOK_UPDATED") {
        window.dispatchEvent(new CustomEvent("book-updated", { detail: msg.payload }));
      }

      // Handle BOOK_DELETED by dispatching a custom event for cache invalidation.
      if (msg.type === "BOOK_DELETED") {
        window.dispatchEvent(new CustomEvent("book-deleted", { detail: msg.payload }));
      }

      // Handle METADATA_PROPOSAL_READY by dispatching a custom event for admin UIs.
      if (msg.type === "METADATA_PROPOSAL_READY") {
        window.dispatchEvent(new CustomEvent("metadata-proposal-ready", { detail: msg.payload }));
      }

      // Handle NOTIFICATION by dispatching a custom event for toast displays.
      if (msg.type === "NOTIFICATION") {
        window.dispatchEvent(new CustomEvent("ws-notification", { detail: msg.payload }));
      }

      dispatch(msg);
    };

    socket.onclose = () => {
      stopPing();
      if (!stopped) {
        scheduleReconnect();
      }
    };

    socket.onerror = () => {
      // onerror is always followed by onclose, so reconnect is handled there.
      stopPing();
    };
  }

  function disconnect(): void {
    stopped = true;
    stopPing();

    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    if (socket) {
      socket.onclose = null;
      socket.onerror = null;
      socket.onmessage = null;
      socket.close();
      socket = null;
    }
  }

  function on(type: string, handler: WSHandler): () => void {
    if (!handlers.has(type)) {
      handlers.set(type, new Set());
    }
    handlers.get(type)!.add(handler);

    return () => {
      handlers.get(type)?.delete(handler);
    };
  }

  function onAny(handler: WSHandler): () => void {
    anyHandlers.add(handler);
    return () => {
      anyHandlers.delete(handler);
    };
  }

  return { connect, disconnect, on, onAny };
}
