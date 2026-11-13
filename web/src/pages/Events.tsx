import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "@nanostores/react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Card, Label, ListBox, Select, Switch } from "@heroui/react";
import { EmptyState } from "@heroui-pro/react";
import type { Key } from "react-aria-components";

function InboxGlyph() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M3 12h4l2 3h6l2-3h4" />
      <path d="M5 6h14l2 6v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6l2-6Z" />
    </svg>
  );
}
import { $feed, $projects } from "../store";
import { fetchEventPage } from "../api/rest";
import { fromRestEvent, type FeedEvent } from "../store/normalize";
import { EVENT_ROW_GRID, EventItem } from "../components/EventRail";
import { EVENT_TYPES, SEVERITIES } from "../protocol/types";
import { eventLabel } from "../lib/flag";

const SEVERITY_LABEL: Record<string, string> = {
  critical: "紧急",
  notable: "重要",
  info: "信息",
  ops: "运维",
};
const EVENT_PAGE_LIMIT = 200;

function one(v: Key | Key[] | null): string {
  if (v == null) return "";
  return String(Array.isArray(v) ? v[0] : v);
}

export function Events() {
  const feed = useStore($feed);
  const projects = useStore($projects);

  const [projectId, setProjectId] = useState(0);
  const [eventType, setEventType] = useState("");
  const [severity, setSeverity] = useState("");
  const [onlyRestock, setOnlyRestock] = useState(false);
  const [history, setHistory] = useState<FeedEvent[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [reloadSeq, setReloadSeq] = useState(0);
  const queryKey = `${projectId}:${eventType}:${severity}:${onlyRestock}`;
  const queryKeyRef = useRef(queryKey);

  // Pull the first page whenever filters change; live WS events are layered on
  // top from $feed while older history is paged by cursor.
  useEffect(() => {
    let alive = true;
    queryKeyRef.current = queryKey;
    setLoading(true);
    setLoadingMore(false);
    setLoadError(false);
    setNextCursor(null);
    setHasMore(false);
    fetchEventPage({
      project_id: projectId || undefined,
      event_type: onlyRestock ? "sku_restock" : eventType || undefined,
      severity: severity || undefined,
      limit: EVENT_PAGE_LIMIT,
    })
      .then((page) => {
        if (!alive) return;
        setHistory(page.events.map(fromRestEvent));
        setNextCursor(page.next_cursor ?? null);
        setHasMore(page.has_more);
      })
      .catch(() => {
        if (!alive) return;
        setHistory([]);
        setLoadError(true);
      })
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [projectId, eventType, severity, onlyRestock, queryKey, reloadSeq]);

  const loadMore = useCallback(() => {
    if (loading || loadingMore || !hasMore || !nextCursor) return;
    const activeKey = queryKeyRef.current;
    setLoadingMore(true);
    setLoadError(false);
    fetchEventPage({
      project_id: projectId || undefined,
      event_type: onlyRestock ? "sku_restock" : eventType || undefined,
      severity: severity || undefined,
      cursor: nextCursor,
      limit: EVENT_PAGE_LIMIT,
    })
      .then((page) => {
        if (queryKeyRef.current !== activeKey) return;
        setHistory((cur) => {
          const seen = new Set(cur.map((e) => e.id));
          const add = page.events.map(fromRestEvent).filter((e) => {
            if (seen.has(e.id)) return false;
            seen.add(e.id);
            return true;
          });
          return [...cur, ...add];
        });
        setNextCursor(page.next_cursor ?? null);
        setHasMore(page.has_more);
      })
      .catch(() => {
        if (queryKeyRef.current === activeKey) setLoadError(true);
      })
      .finally(() => {
        if (queryKeyRef.current === activeKey) setLoadingMore(false);
      });
  }, [eventType, hasMore, loading, loadingMore, nextCursor, onlyRestock, projectId, severity]);

  const rows = useMemo(() => {
    const match = (e: FeedEvent) =>
      (!projectId || e.project_id === projectId) &&
      (!severity || e.severity === severity) &&
      (onlyRestock ? e.event_type === "sku_restock" : !eventType || e.event_type === eventType);
    const live = feed.filter(match);
    const seen = new Set(live.map((e) => e.id));
    return [...live, ...history.filter((e) => match(e) && !seen.has(e.id))].sort((a, b) =>
      b.server_ts !== a.server_ts ? b.server_ts - a.server_ts : b.seq - a.seq,
    );
  }, [feed, history, projectId, eventType, severity, onlyRestock]);

  const parentRef = useRef<HTMLDivElement>(null);
  const loaderRow = hasMore || loadingMore || loadError;
  const virt = useVirtualizer({
    count: rows.length + (loaderRow ? 1 : 0),
    getScrollElement: () => parentRef.current,
    estimateSize: () => 58,
    overscan: 8,
  });
  const virtualItems = virt.getVirtualItems();

  useEffect(() => {
    const last = virtualItems[virtualItems.length - 1];
    if (!last || rows.length === 0) return;
    if (last.index >= rows.length - 1) loadMore();
  }, [loadMore, rows.length, virtualItems]);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Select
          aria-label="项目"
          className="w-48"
          selectionMode="single"
          value={String(projectId)}
          onChange={(v) => setProjectId(Number(one(v)) || 0)}
        >
          <Label className="sr-only">项目</Label>
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              <ListBox.Item id="0" textValue="全部项目">
                全部项目
                <ListBox.ItemIndicator />
              </ListBox.Item>
              {Object.values(projects).map((p) => (
                <ListBox.Item
                  key={p.project_id}
                  id={String(p.project_id)}
                  textValue={p.name?.trim() ? p.name : `项目 #${p.project_id}`}
                >
                  {p.name?.trim() ? p.name : `项目 #${p.project_id}`}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>

        <Select
          aria-label="事件类型"
          className="w-44"
          selectionMode="single"
          isDisabled={onlyRestock}
          value={eventType || "all"}
          onChange={(v) => setEventType(one(v) === "all" ? "" : one(v))}
        >
          <Label className="sr-only">事件类型</Label>
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              <ListBox.Item id="all" textValue="全部类型">
                全部类型
                <ListBox.ItemIndicator />
              </ListBox.Item>
              {EVENT_TYPES.map((t) => (
                <ListBox.Item key={t} id={t} textValue={eventLabel[t] ?? t}>
                  {eventLabel[t] ?? t}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>

        <Select
          aria-label="级别"
          className="w-36"
          selectionMode="single"
          value={severity || "all"}
          onChange={(v) => setSeverity(one(v) === "all" ? "" : one(v))}
        >
          <Label className="sr-only">级别</Label>
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              <ListBox.Item id="all" textValue="全部级别">
                全部级别
                <ListBox.ItemIndicator />
              </ListBox.Item>
              {SEVERITIES.map((s) => (
                <ListBox.Item key={s} id={s} textValue={SEVERITY_LABEL[s] ?? s}>
                  {SEVERITY_LABEL[s] ?? s}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>

        <Switch isSelected={onlyRestock} onChange={setOnlyRestock}>
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
            只看回流
          </Switch.Content>
        </Switch>

        <span className="ml-auto text-xs text-muted">
          {rows.length} 条{hasMore ? "+" : ""}
        </span>
      </div>

      <Card className="overflow-hidden p-0">
        <div
          className={`${EVENT_ROW_GRID} border-b border-border bg-surface-secondary py-2 text-[11px] font-medium text-muted`}
        >
          <span>级别</span>
          <span>事件</span>
          <span>项目</span>
          <span className="text-right">时间</span>
        </div>
        <div ref={parentRef} className="h-[calc(100vh-12rem)] overflow-y-auto">
          {loading && rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-muted">加载事件中…</div>
          ) : loadError && rows.length === 0 ? (
            <div className="py-12 text-center text-sm text-muted">
              <button className="text-link hover:underline" type="button" onClick={() => setReloadSeq((n) => n + 1)}>
                加载失败，点击重试
              </button>
            </div>
          ) : rows.length === 0 ? (
            <div className="py-12">
              <EmptyState>
                <EmptyState.Header>
                  <EmptyState.Media variant="icon">
                    <InboxGlyph />
                  </EmptyState.Media>
                  <EmptyState.Title>没有匹配的事件</EmptyState.Title>
                  <EmptyState.Description>调整项目、类型或级别筛选，或等待新的实时事件。</EmptyState.Description>
                </EmptyState.Header>
              </EmptyState>
            </div>
          ) : (
            <div style={{ height: virt.getTotalSize(), position: "relative", width: "100%" }}>
              {virtualItems.map((vi) => {
                const row = rows[vi.index];
                return (
                  <div
                    key={row ? row.id + ":" + row.seq : "events-loader"}
                    style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${vi.start}px)` }}
                  >
                    {row ? (
                      <EventItem e={row} />
                    ) : (
                      <div className="border-b border-border px-3 py-3 text-center text-xs text-muted">
                        {loadError ? (
                          <button className="text-link hover:underline" type="button" onClick={loadMore}>
                            加载失败，点击重试
                          </button>
                        ) : loadingMore ? (
                          "加载更多事件中…"
                        ) : (
                          "继续向下滚动加载更早事件"
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
