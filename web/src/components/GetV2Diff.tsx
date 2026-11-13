import type { FieldChange } from "../lib/fieldChanges";

// GetV2Diff renders changed getV2 fields as a compact, monospace old→new list
// with git-diff coloring (rose old / emerald new — same palette as lib/flag's
// statusColor). It lives inside the event HoverCard; callers only mount it when
// fieldChanges() returned a non-empty list.
export function GetV2Diff({ changes }: { changes: FieldChange[] }) {
  return (
    <div className="min-w-[15rem] font-mono text-xs">
      <div className="mb-1.5 text-[10px] uppercase tracking-wide text-muted/70">getV2 字段变化</div>
      <div className="space-y-1">
        {changes.map((c) => (
          <div key={c.key} className="grid grid-cols-[auto_1fr] items-baseline gap-x-3">
            <span className="text-muted">{c.key}</span>
            <span className="tabular-nums">
              <span className="text-rose-400 line-through decoration-rose-400/40">{c.oldText}</span>
              <span className="mx-1.5 text-muted/60">→</span>
              <span className="text-emerald-400">{c.newText}</span>
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
