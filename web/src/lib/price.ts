// Prices arrive in fen (分); display divides by 100 (docs/bilibili-getv2-monitoring.md §5.4).

export function yuan(fen: number): string {
  // Round to cents, then let String drop trailing zeros (128.50 → "128.5").
  return String(Math.round(fen) / 100);
}

// priceBand renders a low–high range, collapsing equal bounds and handling the
// no-data (0/0) case projects show before a node has filled them in. A missing
// bound (0) falls back to the other so we never render "¥0–…".
export function priceBand(low: number, high: number): string {
  if (low <= 0 && high <= 0) return "—";
  const lo = low > 0 ? low : high;
  const hi = high > 0 ? high : low;
  if (lo === hi) return `¥${yuan(lo)}`;
  return `¥${yuan(lo)}–${yuan(hi)}`;
}
