// Bootstrap wiring: REST first paint + feed backfill, then the live WS, plus a
// low-frequency REST reconcile for project aggregates the WS doesn't push.
import { fetchEvents, fetchSnapshot } from "../api/rest";
import { connectLive } from "../api/ws";
import { startNotifications } from "../lib/notify";
import { applySnapshot, backfillEvents } from "./index";

const RECONCILE_MS = 15000;

export interface BootstrapHandle {
  stop: () => void;
}

export function bootstrap(): BootstrapHandle {
  let stopped = false;

  // Pull the REST-only aggregates (e.g. last_poll_ts liveness) the WS never pushes.
  const reconcileNow = (): void => {
    if (stopped) return;
    fetchSnapshot()
      .then(applySnapshot)
      .catch(() => {
        /* transient; next tick / trigger retries */
      });
  };

  // First paint + recent-events backfill (best-effort; WS will fill in the rest).
  void (async () => {
    try {
      applySnapshot(await fetchSnapshot());
    } catch {
      /* WS will recover */
    }
    try {
      backfillEvents(await fetchEvents({ limit: 100 }));
    } catch {
      /* feed starts empty */
    }
  })();

  // Resync on every (re)connect — a reconnect after a deploy/network blip refreshes
  // liveness immediately instead of waiting out the interval.
  const stopLive = connectLive(reconcileNow);
  const stopNotify = startNotifications();

  const reconcile = setInterval(reconcileNow, RECONCILE_MS);

  // Backgrounded tabs get setInterval throttled to >=1/min, so last_poll_ts (only
  // delivered by this reconcile) can drift minutes stale and the timeline looks
  // stalled. Snap fresh the moment the tab is focused again.
  const onVisible = (): void => {
    if (document.visibilityState === "visible") reconcileNow();
  };
  document.addEventListener("visibilitychange", onVisible);

  return {
    stop: () => {
      stopped = true;
      clearInterval(reconcile);
      document.removeEventListener("visibilitychange", onVisible);
      stopLive();
      stopNotify();
    },
  };
}
