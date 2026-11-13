import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useStore } from "@nanostores/react";
import { Card, Label, ScrollShadow, SearchField } from "@heroui/react";
import { EmptyState } from "@heroui-pro/react";

// Minimal inline glyph for empty states (the project has no icon dependency).
function InboxGlyph() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M3 12h4l2 3h6l2-3h4" />
      <path d="M5 6h14l2 6v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6l2-6Z" />
    </svg>
  );
}
import { $feed, $nodes, $projects, $screens, $skus, projectScreens, projectSkus } from "../store";
import { anyBuyable, projectSortRank } from "../store/normalize";
import { ProjectBlock, projectTitle } from "../components/StatusTable";
import { EventRail } from "../components/EventRail";
import { Stat } from "../components/primitives";

export function Overview() {
  const projects = useStore($projects);
  const screens = useStore($screens);
  const skus = useStore($skus);
  const nodes = useStore($nodes);
  const feed = useStore($feed);

  const [q, setQ] = useState("");

  const sidebarFeed = useMemo(
    () => feed.filter((e) => e.event_type !== "node_online" && e.event_type !== "node_offline").slice(0, 80),
    [feed],
  );

  const list = useMemo(() => {
    const query = q.trim().toLowerCase();
    return Object.values(projects)
      .map((p) => ({ p, skus: projectSkus(p.project_id, skus) }))
      .filter(({ p }) => {
        if (query && !projectTitle(p).toLowerCase().includes(query) && !String(p.project_id).includes(query))
          return false;
        return true;
      })
      .sort((a, b) => {
        const ra = projectSortRank(a.p, a.skus);
        const rb = projectSortRank(b.p, b.skus);
        if (ra !== rb) return ra - rb;
        return b.p.wish_count - a.p.wish_count;
      });
  }, [projects, skus, q]);

  const kpi = useMemo(() => {
    const all = Object.values(projects);
    const onsale = all.filter((p) => anyBuyable(projectSkus(p.project_id, skus))).length;
    const hot = all.filter((p) => p.hot).length;
    const nodeList = Object.values(nodes);
    const online = nodeList.filter((n) => n.online).length;
    return { total: all.length, onsale, hot, online, nodes: nodeList.length };
  }, [projects, skus, nodes]);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="监控项目" value={kpi.total} />
        <Stat label="在售可买" value={kpi.onsale} tone={kpi.onsale > 0 ? "success" : undefined} />
        <Stat label="热门" value={kpi.hot} tone={kpi.hot > 0 ? "warning" : undefined} />
        <Stat label="在线节点" value={`${kpi.online}/${kpi.nodes}`} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <SearchField aria-label="搜索项目" value={q} onChange={setQ} className="w-60">
              <Label className="sr-only">搜索项目</Label>
              <SearchField.Group>
                <SearchField.SearchIcon />
                <SearchField.Input placeholder="搜索项目名 / ID" />
                <SearchField.ClearButton />
              </SearchField.Group>
            </SearchField>

            <span className="ml-auto text-xs text-muted">{list.length} 个项目</span>
          </div>

          {list.length === 0 ? (
            <Card className="py-10">
              <EmptyState>
                <EmptyState.Header>
                  <EmptyState.Media variant="icon">
                    <InboxGlyph />
                  </EmptyState.Media>
                  <EmptyState.Title>
                    {Object.keys(projects).length === 0 ? "等待数据" : "没有匹配的项目"}
                  </EmptyState.Title>
                  <EmptyState.Description>
                    {Object.keys(projects).length === 0
                      ? "尚未收到任何项目，确认 master 已连接并有节点上报。"
                      : "调整搜索关键字再试。"}
                  </EmptyState.Description>
                </EmptyState.Header>
              </EmptyState>
            </Card>
          ) : (
            list.map(({ p }) => (
              <ProjectBlock
                key={p.project_id}
                project={p}
                screens={projectScreens(p.project_id, screens)}
                skus={skus}
              />
            ))
          )}
        </div>

        <aside className="lg:sticky lg:top-[4.5rem] lg:h-[calc(100vh-6rem)]">
          <Card className="flex h-full flex-col overflow-hidden p-0">
            <div className="flex items-center justify-between border-b border-separator/60 px-3 py-2.5">
              <span className="text-sm font-semibold text-foreground">实时事件</span>
              <Link to="/events" className="text-xs text-link hover:underline">
                查看全部 →
              </Link>
            </div>
            <ScrollShadow className="flex-1" hideScrollBar>
              <EventRail events={sidebarFeed} emptyHint="等待实时事件…" />
            </ScrollShadow>
          </Card>
        </aside>
      </div>
    </div>
  );
}
