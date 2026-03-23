import {
  type Component,
  type JSX,
  createContext,
  createEffect,
  onCleanup,
  useContext,
} from "solid-js";
import { createWebSocketClient, type WSClient, type WSHandler } from "./socket";
import { getAccessToken } from "../api/client";

const WSContext = createContext<WSClient>();

/**
 * WSProvider creates a WebSocket client and connects it when the user is
 * authenticated. It disconnects on logout.
 *
 * Usage: wrap the authenticated portion of the app with <WSProvider>.
 * Children can call useWS() to get the client.
 *
 * The isAuthenticated prop should be a reactive signal accessor so that
 * WSProvider can react to auth state changes.
 */
const WSProvider: Component<{
  /** Reactive accessor — returns true when the user is logged in. */
  isAuthenticated: () => boolean;
  children: JSX.Element;
}> = (props) => {
  const client = createWebSocketClient(getAccessToken);

  // Connect when authenticated, disconnect when not.
  createEffect(() => {
    if (props.isAuthenticated()) {
      client.connect();
    } else {
      client.disconnect();
    }
  });

  onCleanup(() => {
    client.disconnect();
  });

  return <WSContext.Provider value={client}>{props.children}</WSContext.Provider>;
};

/**
 * useWS returns the WebSocket client from context.
 * Must be called inside a WSProvider.
 */
export function useWS(): WSClient {
  const ctx = useContext(WSContext);
  if (!ctx) {
    throw new Error("useWS must be used inside WSProvider");
  }
  return ctx;
}

/**
 * useWSEvent subscribes to a specific WebSocket message type.
 * The subscription is automatically cleaned up when the component unmounts.
 */
export function useWSEvent(type: string, handler: WSHandler): void {
  const client = useWS();

  createEffect(() => {
    const unsubscribe = client.on(type, handler);
    onCleanup(unsubscribe);
  });
}

export default WSProvider;
