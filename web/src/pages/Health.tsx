import { useStore } from "@nanostores/react";
import { Chip } from "@heroui/react";
import { DataGrid, EmptyState, type DataGridColumn } from "@heroui-pro/react";
import { $nodes } from "../store";
import type { RestNode } from "../api/rest";
import { formatAge } from "../lib/time";
import { useTick } from "../lib/hooks";
import { Stat } from "../components/primitives";

function ServerGlyph() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="4" width="18" height="6" rx="1" />
      <rect x="3" y="14" width="18" height="6" rx="1" />
      <path d="M7 7h.01M7 17h.01" />
    </svg>
  );
}

function LastSeenCell({ lastSeen }: { lastSeen: number }) {
  useTick(1000);
  return <>{formatAge(lastSeen)}</>;
}

const columns: DataGridColumn<RestNode>[] = [
  {
    id: "node",
    header: "节点",
    accessorKey: "node_id",
    isRowHeader: true,
    cell: (n) => (
      <span>
        <span className="text-foreground">{n.node_name || n.node_id}</span>
        <span className="ml-1.5 text-xs text-muted/70">{n.node_id}</span>
      </span>
    ),
  },
  {
    id: "region",
    header: "区域",
    accessorKey: "region",
    cell: (n) => n.region || "—",
  },
  {
    id: "status",
    header: "状态",
    accessorKey: "online",
    cell: (n) => (
      <Chip color={n.online ? "success" : "danger"} variant="soft" size="sm">
        {n.online ? "在线" : "离线"}
      </Chip>
    ),
  },
  {
    id: "in_flight",
    header: "在途",
    accessorKey: "in_flight",
    align: "end",
    cellClassName: "tabular-nums",
  },
  {
    id: "soft",
    header: "软限流 429",
    accessorKey: "soft_limited",
    align: "end",
    cellClassName: "tabular-nums",
    cell: (n) => (
      <span className={n.soft_limited > 0 ? "text-warning" : "text-muted"}>{n.soft_limited}</span>
    ),
  },
  {
    id: "hard",
    header: "硬封 412",
    accessorKey: "hard_blocked",
    align: "end",
    cellClassName: "tabular-nums",
    cell: (n) => (
      <span className={n.hard_blocked > 0 ? "text-danger" : "text-muted"}>{n.hard_blocked}</span>
    ),
  },
  {
    id: "last_seen",
    header: "最后心跳",
    accessorKey: "last_seen",
    align: "end",
    cellClassName: "tabular-nums text-muted",
    cell: (n) => <LastSeenCell lastSeen={n.last_seen} />,
  },
];

export function Health() {
  const nodes = useStore($nodes);
  // Stable order by node ID — a live health table shouldn't reorder under the
  // user, so the grid is intentionally not sortable.
  const list = Object.values(nodes).sort((a, b) => a.node_id.localeCompare(b.node_id));
  const online = list.filter((n) => n.online).length;
  const soft = list.reduce((a, n) => a + n.soft_limited, 0);
  const hard = list.reduce((a, n) => a + n.hard_blocked, 0);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat
          label="在线节点"
          value={`${online}/${list.length}`}
          tone={online < list.length ? "warning" : "success"}
        />
        <Stat label="在途请求" value={list.reduce((a, n) => a + n.in_flight, 0)} />
        <Stat label="软限流 429" value={soft} tone={soft > 0 ? "warning" : undefined} />
        <Stat label="硬封禁 412" value={hard} tone={hard > 0 ? "danger" : undefined} />
      </div>

      <DataGrid
        aria-label="采集节点"
        columns={columns}
        data={list}
        getRowId={(n) => n.node_id}
        renderEmptyState={() => (
          <div className="py-6">
            <EmptyState size="sm">
              <EmptyState.Header>
                <EmptyState.Media variant="icon">
                  <ServerGlyph />
                </EmptyState.Media>
                <EmptyState.Title>暂无节点</EmptyState.Title>
                <EmptyState.Description>等待采集节点上报心跳。</EmptyState.Description>
              </EmptyState.Header>
            </EmptyState>
          </div>
        )}
      />
    </div>
  );
}
