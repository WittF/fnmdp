// The status-grid time toolbar, lifted out of StatusGrid so one control can drive
// many grids (global) or a single aggregation group. The granularity preset IS the
// window span, so in live mode we only show it + a 实时 chip (no redundant clock
// range); the absolute range appears only when panned into history.
import type { ReactNode } from "react";
import { Button, ButtonGroup, Chip, Dropdown, Label } from "@heroui/react";
import type { Selection } from "@heroui/react";
import type { Win } from "../lib/statusWindow";
import {
  gridStatusLabel,
  gridStatusVar,
  type GridStatus,
  trajectoryWindow,
  WINDOW_PRESETS,
} from "../lib/statusgrid";
import { formatClock, serverNow } from "../lib/time";
import { useTick } from "../lib/hooks";

const LEGEND: GridStatus[] = ["available", "soldout", "presale", "ended", "nodata"];
const hm = (ts: number) => formatClock(ts).slice(0, 5); // HH:MM

function ChevronDownIcon() {
  return (
    <svg aria-hidden width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}
function ChevronLeftIcon() {
  return (
    <svg aria-hidden width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}
function ChevronRightIcon() {
  return (
    <svg aria-hidden width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}

export function StatusLegend() {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted">
      {LEGEND.map((s) => (
        <span key={s} className="inline-flex items-center gap-1">
          <span
            className="h-2.5 w-2.5 rounded-[3px]"
            style={{ backgroundColor: gridStatusVar[s], opacity: s === "nodata" ? 0.4 : 1 }}
          />
          {gridStatusLabel[s]}
        </span>
      ))}
    </div>
  );
}

function GranularityMenu({ win, onWindow }: { win: Win; onWindow: (ms: number) => void }) {
  const current = WINDOW_PRESETS.find((w) => w.intervalMs === win.intervalMs)?.label ?? "自定义";
  return (
    <Dropdown>
      <Dropdown.Trigger
        className="button button--sm button--secondary inline-flex h-7 shrink-0 items-center gap-1 whitespace-nowrap px-2 tabular-nums"
        aria-label="时间窗口"
      >
        {current}
        <ChevronDownIcon />
      </Dropdown.Trigger>
      <Dropdown.Popover className="min-w-28">
        <Dropdown.Menu
          selectionMode="single"
          selectedKeys={new Set([String(win.intervalMs)])}
          onSelectionChange={(keys: Selection) => {
            if (keys === "all") return;
            const k = [...keys][0];
            if (k != null) onWindow(Number(k));
          }}
        >
          {WINDOW_PRESETS.map((w) => (
            <Dropdown.Item key={w.intervalMs} id={String(w.intervalMs)} textValue={w.label}>
              <Dropdown.ItemIndicator />
              <Label>{w.label}</Label>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}

export function StatusWindowControl({
  win,
  onWindow,
  onPan,
  onReset,
  trailing,
}: {
  win: Win;
  onWindow: (intervalMs: number) => void;
  onPan: (dir: -1 | 1) => void;
  onReset: () => void;
  trailing?: ReactNode;
}) {
  const live = win.anchorMs === null;
  useTick(live ? 15_000 : 0); // keep the live range fresh

  const endMs = win.anchorMs ?? serverNow() * 1000;
  const { sinceMs } = trajectoryWindow(win.intervalMs, endMs);

  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted">
      <span className="text-muted/60">窗口</span>
      <GranularityMenu win={win} onWindow={onWindow} />
      <ButtonGroup variant="tertiary" size="sm">
        <Button isIconOnly className="h-7 min-w-7" onPress={() => onPan(-1)} aria-label="更早一段">
          <ChevronLeftIcon />
        </Button>
        <Button isIconOnly className="h-7 min-w-7" isDisabled={live} onPress={() => onPan(1)} aria-label="更新一段">
          <ButtonGroup.Separator />
          <ChevronRightIcon />
        </Button>
      </ButtonGroup>
      {live ? (
        <Chip color="success" size="sm" variant="soft">
          实时
        </Chip>
      ) : (
        <>
          <span className="tabular-nums text-foreground/70">
            {hm(sinceMs / 1000)} → {hm(endMs / 1000)}
          </span>
          <Button size="sm" variant="ghost" className="h-7 px-2" onPress={onReset}>
            回到实时
          </Button>
        </>
      )}
      {trailing}
    </div>
  );
}
