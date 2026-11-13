import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useStore } from "@nanostores/react";
import { Card, ScrollShadow, Separator } from "@heroui/react";
import { $feed, $projects, skuKey } from "../store";
import { fetchEvents } from "../api/rest";
import { fromRestEvent, type FeedEvent } from "../store/normalize";
import { projectTitle } from "../components/StatusTable";
import { EventRail } from "../components/EventRail";
import { EventFilterButton } from "../components/EventFilterButton";
import { $eventFilter, isVisible } from "../lib/eventFilter";
import { SkuBoard } from "../components/SkuBoard";
import { MallLinkButton } from "../components/MallLinkButton";
import { focusSku } from "../lib/skuLayout";
import { resetAnchors } from "../lib/statusWindow";
import { AgeStamp, Countdown, HotBadge, PriceBand, StatusBadge, WishCount } from "../components/primitives";
import { flagLabel, statusKind } from "../lib/flag";

function ChevronLeftIcon() {
  return (
    <svg
      aria-hidden
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

// Back-to-wall affordance: a subordinate ghost link (muted → emphasized on hover)
// rather than an underline, with a proper chevron icon in place of a literal glyph.
function BackLink() {
  return (
    <Link
      to="/"
      className="-ml-2 inline-flex w-fit items-center gap-1.5 rounded-md px-2 py-1 text-sm text-muted transition-colors hover:bg-surface-hover hover:text-foreground"
    >
      <ChevronLeftIcon />
      返回状态墙
    </Link>
  );
}

export function ProjectDetail() {
  const { id } = useParams();
  const pid = Number(id);
  const projects = useStore($projects);
  const feed = useStore($feed);
  const filter = useStore($eventFilter);
  const [history, setHistory] = useState<FeedEvent[]>([]);

  const project = projects[String(pid)];

  useEffect(() => {
    let alive = true;
    fetchEvents({ project_id: pid, limit: 100 })
      .then((rows) => alive && setHistory(rows.map(fromRestEvent)))
      .catch(() => alive && setHistory([]));
    return () => {
      alive = false;
    };
  }, [pid]);

  // Switching projects returns the shared status-grid window to live + drops any
  // group overrides (group ids are scoped to the previous project's layout).
  useEffect(() => {
    resetAnchors();
  }, [pid]);

  // Timeline = REST history merged with live feed for this project, deduped.
  const timeline = useMemo(() => {
    const live = feed.filter((e) => e.project_id === pid);
    const seen = new Set(live.map((e) => e.id));
    const merged = [...live, ...history.filter((e) => !seen.has(e.id))];
    return merged.sort((a, b) => (b.server_ts !== a.server_ts ? b.server_ts - a.server_ts : b.seq - a.seq));
  }, [feed, history, pid]);

  // Apply the global event-type filter before rendering (keeps EventRail stateless).
  const visibleTimeline = useMemo(
    () => timeline.filter((e) => isVisible(e.event_type, filter)),
    [timeline, filter],
  );

  if (!project) {
    return (
      <div className="space-y-3">
        <BackLink />
        <Card className="px-4 py-16 text-center text-sm text-muted">项目 #{pid} 暂无数据</Card>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <BackLink />

      <Card>
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <h1 className="text-card-title font-semibold text-foreground">{projectTitle(project)}</h1>
            <HotBadge hot={project.hot} />
            <StatusBadge kind={statusKind(project.flag, true)} text={flagLabel(project.flag)} />
          </div>
          <MallLinkButton pid={pid} />
        </div>
        <Separator />
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <PriceBand
            low={project.salable_low || project.price_low}
            high={project.salable_high || project.price_high}
          />
          <WishCount value={project.wish_count} />
          <Countdown saleStart={project.sale_start} />
          <AgeStamp ts={project.last_event_ts ?? 0} label="最后变化" />
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
        <SkuBoard pid={pid} />

        <aside className="lg:sticky lg:top-[4.5rem] lg:h-[calc(100vh-6rem)]">
          <Card className="flex flex-col overflow-hidden p-0 lg:h-full">
            <div className="flex items-center justify-between gap-2 border-b border-separator/60 px-3 py-2.5">
              <span className="text-sm font-semibold text-foreground">事件时间线</span>
              <EventFilterButton />
            </div>
            <ScrollShadow className="max-h-[60vh] flex-1 lg:max-h-none" hideScrollBar>
              <EventRail
                events={visibleTimeline}
                emptyHint={timeline.length > 0 ? "当前筛选下暂无事件" : "该项目暂无事件"}
                onFocus={(e) => focusSku(pid, skuKey(pid, e.screen_id, e.sku_id), e.server_ts * 1000)}
              />
            </ScrollShadow>
          </Card>
        </aside>
      </div>
    </div>
  );
}
