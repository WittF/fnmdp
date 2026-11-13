// /live WebSocket client: connects, dispatches increments into the store, and
// reconnects with exponential backoff (1s→30s). The connect-time `snapshot`
// message only carries projects+nodes (no screens/skus), so its content is
// ignored — REST /api/snapshot is the source of truth; we only read its server_ts.
import type { Message } from "../protocol/types";
import { $conn, applyNodeStatus, applyStateUpdate, pushWsEvent } from "../store";
import { syncServerClock } from "../lib/time";

function liveURL(): string {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}/live`;
}

function dispatch(msg: Message): void {
  switch (msg.type) {
    case "snapshot":
      syncServerClock(msg.server_ts);
      break;
    case "state_update":
      applyStateUpdate(msg);
      break;
    case "event":
      pushWsEvent(msg);
      break;
    case "node_status":
      applyNodeStatus(msg);
      break;
    default:
      break;
  }
}

// connectLive opens the socket and keeps it alive. onConnect fires on every
// successful (re)open so callers can resync REST-only state (e.g. last_poll_ts)
// right after a reconnect instead of waiting for the next reconcile tick. Returns
// a disposer that stops reconnection and closes the socket.
export function connectLive(onConnect?: () => void): () => void {
  let ws: WebSocket | null = null;
  let backoff = 1000;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;
  let everConnected = false;

  const open = (): void => {
    if (stopped) return;
    $conn.set(everConnected ? "reconnecting" : "connecting");
    ws = new WebSocket(liveURL());

    ws.onopen = () => {
      everConnected = true;
      backoff = 1000;
      $conn.set("online");
      onConnect?.();
    };
    ws.onmessage = (e) => {
      try {
        dispatch(JSON.parse(e.data) as Message);
      } catch {
        // ignore malformed frames
      }
    };
    ws.onclose = () => {
      if (stopped) return;
      $conn.set("reconnecting");
      timer = setTimeout(open, backoff);
      backoff = Math.min(backoff * 2, 30000);
    };
    ws.onerror = () => ws?.close();
  };

  open();

  return () => {
    stopped = true;
    if (timer) clearTimeout(timer);
    $conn.set("offline");
    ws?.close();
  };
}
