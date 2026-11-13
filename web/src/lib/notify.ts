// Opt-in browser notifications + sound. Fires only while the tab is visible (no
// background Web Push). Subscription prefs live in localStorage, no
// backend. Defaults cover the high-value signals (restock / onsale / window open /
// new SKU / low stock).
import { atom } from "nanostores";
import type { EventType } from "../protocol/types";
import { $projects, onNewEvent } from "../store";
import type { FeedEvent } from "../store/normalize";
import { eventLabel } from "./flag";

export interface Subs {
  enabled: boolean;
  sound: boolean;
  types: Partial<Record<EventType, boolean>>;
  projectIds: number[]; // empty = all projects
}

export const DEFAULT_TYPES: EventType[] = [
  "sku_restock",
  "sku_onsale",
  "project_window_open",
  "sku_added",
  "sku_low_stock",
];

// Event types offered in the settings UI (curated; ops/info noise left out).
export const NOTIFY_CHOICES: EventType[] = [
  "sku_restock",
  "sku_onsale",
  "sku_added",
  "sku_low_stock",
  "project_window_open",
  "sku_soldout",
  "sku_price_change",
];

const KEY = "fnmdp.subs.v1";

function load(): Subs {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) return JSON.parse(raw) as Subs;
  } catch {
    /* fall through to defaults */
  }
  const types: Partial<Record<EventType, boolean>> = {};
  for (const t of DEFAULT_TYPES) types[t] = true;
  return { enabled: false, sound: true, types, projectIds: [] };
}

export const $subs = atom<Subs>(load());
$subs.subscribe((s) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(s));
  } catch {
    /* private mode / quota — prefs just won't persist */
  }
});

export function setSubs(patch: Partial<Subs>): void {
  $subs.set({ ...$subs.get(), ...patch });
}

export function toggleType(t: EventType): void {
  const s = $subs.get();
  setSubs({ types: { ...s.types, [t]: !s.types[t] } });
}

export function permission(): NotificationPermission {
  return "Notification" in window ? Notification.permission : "denied";
}

export async function requestPermission(): Promise<boolean> {
  if (!("Notification" in window)) return false;
  if (Notification.permission === "granted") return true;
  return (await Notification.requestPermission()) === "granted";
}

let audioCtx: AudioContext | null = null;
function beep(): void {
  try {
    const Ctor = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = audioCtx ?? new Ctor();
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();
    osc.connect(gain);
    gain.connect(audioCtx.destination);
    osc.type = "sine";
    osc.frequency.value = 880;
    gain.gain.setValueAtTime(0.06, audioCtx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.0001, audioCtx.currentTime + 0.25);
    osc.start();
    osc.stop(audioCtx.currentTime + 0.25);
  } catch {
    /* autoplay blocked until a user gesture — silent is fine */
  }
}

function matches(e: FeedEvent, s: Subs): boolean {
  if (!s.enabled) return false;
  if (!s.types[e.event_type]) return false;
  if (s.projectIds.length > 0 && !s.projectIds.includes(e.project_id)) return false;
  return true;
}

function body(e: FeedEvent): string {
  const p = $projects.get()[String(e.project_id)];
  const name = p?.name?.trim() ? p.name : `项目 #${e.project_id}`;
  if (e.new && e.new.num > 0) return `${name} · 余 ${e.new.num} 张`;
  return name;
}

// startNotifications wires the feed → notification bridge. Returns an unsubscribe.
export function startNotifications(): () => void {
  return onNewEvent((e) => {
    const s = $subs.get();
    if (!matches(e, s)) return;
    if (typeof document !== "undefined" && document.visibilityState !== "visible") return;
    if (permission() !== "granted") return;
    try {
      new Notification(eventLabel[e.event_type] ?? e.event_type, { body: body(e), tag: e.id });
    } catch {
      /* ignore */
    }
    if (s.sound) beep();
  });
}
