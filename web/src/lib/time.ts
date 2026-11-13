// All countdowns/timestamps use the server clock as the reference, never the
// local clock — collection machines can drift and we must not mis-report a sale
// opening (docs/bilibili-getv2-monitoring.md §5.3). We keep a single offset (serverSec - localSec), refreshed
// from every payload that carries a server_ts, and a local 1s tick reads
// serverNow() each frame. Timestamps are in SECONDS.

let offsetSec = 0;
let offsetInit = false;

export function syncServerClock(serverTs: number): void {
  if (serverTs <= 0) return;
  const candidate = serverTs - Math.floor(Date.now() / 1000);
  // A hot project fanned across many nodes delivers state_updates whose server_ts
  // (Bilibili current_time at each node's fetch) varies by up to a poll interval and
  // arrives out of order. Overwriting the offset with each sample drags serverNow()
  // backward — the timeline visibly retreats by ~tens of seconds. Ratchet forward to
  // the freshest sample so the clock is monotonic; accept a downward jump only when
  // it is large (>5min), which signals a genuine desync (e.g. the local clock was
  // changed) rather than per-poll jitter.
  if (!offsetInit || candidate > offsetSec || offsetSec - candidate > 300) {
    offsetSec = candidate;
    offsetInit = true;
  }
}

export function serverNow(): number {
  return Math.floor(Date.now() / 1000) + offsetSec;
}

export function clockOffset(): number {
  return offsetSec;
}

// _setOffsetForTest pins the offset so countdown logic is deterministic in tests.
export function _setOffsetForTest(s: number): void {
  offsetSec = s;
  offsetInit = true;
}

// secondsUntil returns server-relative seconds until a target timestamp (0 floor).
export function secondsUntil(targetTs: number): number {
  if (targetTs <= 0) return 0;
  return Math.max(0, targetTs - serverNow());
}

function pad(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

// formatCountdown renders remaining seconds as `Nd HH:MM:SS` (days only when >0).
export function formatCountdown(sec: number): string {
  if (sec <= 0) return "00:00:00";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const hms = `${pad(h)}:${pad(m)}:${pad(s)}`;
  return d > 0 ? `${d}天 ${hms}` : hms;
}

const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

// formatDate renders a session start time (seconds) as `YYYY-MM-DD 周X`.
export function formatDate(ts: number): string {
  if (ts <= 0) return "";
  const d = new Date(ts * 1000);
  const ymd = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  return `${ymd} ${WEEKDAYS[d.getDay()]}`;
}

export function datePart(label: string): string {
  return label.split(" ")[0] ?? "";
}

export function shouldShowDateBesideName(name: string, dateLabel: string): boolean {
  if (!dateLabel) return false;
  const n = name.trim();
  if (!n) return true;
  const ymd = datePart(dateLabel);
  return !n.includes(dateLabel) && (!ymd || !n.includes(ymd));
}

// formatClock renders a server timestamp (seconds) as a local HH:MM:SS for feeds.
export function formatClock(ts: number): string {
  if (ts <= 0) return "";
  const dt = new Date(ts * 1000);
  return `${pad(dt.getHours())}:${pad(dt.getMinutes())}:${pad(dt.getSeconds())}`;
}

export function formatDateTime(ts: number): string {
  if (ts <= 0) return "";
  return `${formatDate(ts)} ${formatClock(ts)}`;
}

export function formatAge(ts: number): string {
  if (ts <= 0) return "—";
  const s = Math.max(0, serverNow() - ts);
  if (s < 60) return `${s}s 前`;
  if (s < 3600) return `${Math.floor(s / 60)}m 前`;
  if (s < 86400) return `${Math.floor(s / 3600)}h 前`;
  return `${Math.floor(s / 86400)}d 前`;
}
